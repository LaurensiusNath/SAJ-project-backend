package invoice

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nathan/cnc-pm-backend/internal/dateonly"
)

// noopRequireAdmin: pengganti requireAdmin sungguhan (auth.RequireRole) di
// test package ini - package invoice sengaja tidak import internal/auth
// (lihat main.go/settings.Handler soal role middleware disuntik dari luar),
// jadi tidak ada middleware role nyata untuk dipasang di sini. Sama seperti
// RequireAuth yang juga tidak dipasang di rg pada test ini, pengecekan
// role/auth sungguhan adalah tanggung jawab main.go + internal/auth, bukan
// package ini - test di sini fokus ke validasi & alur Service/Handler.
func noopRequireAdmin(c *gin.Context) { c.Next() }

func newTestRouter() (*gin.Engine, *fakeRepository) {
	repo := newFakeRepository()
	svc := NewService(repo)
	h := NewHandler(svc)
	r := gin.New()
	rg := r.Group("/api/v1")
	h.RegisterRoutes(rg, noopRequireAdmin)
	return r, repo
}

// TestGetByJob_Found proves GET /jobs/{id}/invoice returns the invoice in
// the exact same shape as POST /jobs/{id}/invoice (docs/api-contract.md) -
// a plain Invoice object, not wrapped/renamed.
func TestGetByJob_Found(t *testing.T) {
	router, repo := newTestRouter()
	jobID := uuid.New()
	created, err := NewService(repo).CreateFromJob(context.Background(), jobID, CreateInput{})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+jobID.String()+"/invoice", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var decoded struct {
		Success bool `json:"success"`
		Data    struct {
			ID    string `json:"id"`
			JobID string `json:"job_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &decoded))
	assert.True(t, decoded.Success)
	assert.Equal(t, created.ID.String(), decoded.Data.ID)
	assert.Equal(t, jobID.String(), decoded.Data.JobID)
}

// TestGetByJob_NotFound proves a job with no invoice yet returns 404, not
// 200 with a null/empty body - frontend uses this to distinguish "not
// invoiced yet" from "request failed".
func TestGetByJob_NotFound(t *testing.T) {
	router, _ := newTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+uuid.New().String()+"/invoice", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetByJob_InvalidJobID(t *testing.T) {
	router, _ := newTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/not-a-uuid/invoice", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestGetByJob_DueDate_SerializesAsDateOnly is the JSON-body-literal
// regression guard for the date-serialization bug (docs/api-contract.md
// Catatan Desain: "Kenapa field bertipe tanggal sekarang serialize sebagai
// YYYY-MM-DD..."): invoices.due_date must appear as "YYYY-MM-DD" in the
// ACTUAL marshaled HTTP response body - a Go-level *dateonly.Date/*time.Time
// assertion (like TestService_MarkOverdue elsewhere in this package) can't
// catch this class of bug, since it never inspects the JSON bytes.
func TestGetByJob_DueDate_SerializesAsDateOnly(t *testing.T) {
	router, repo := newTestRouter()
	jobID := uuid.New()
	dueDate := dateonly.New(2026, time.August, 15)
	_, err := NewService(repo).CreateFromJob(context.Background(), jobID, CreateInput{DueDate: &dueDate})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+jobID.String()+"/invoice", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"due_date":"2026-08-15"`, "must be literal YYYY-MM-DD in the response body")
	assert.NotContains(t, body, "T00:00:00Z", "must NOT be RFC3339 - this is exactly the regression this test guards against")
}
