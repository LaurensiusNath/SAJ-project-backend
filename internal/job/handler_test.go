package job

import (
	"context"
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

func newTestRouter(repo Repository) *gin.Engine {
	svc := newTestService(repo)
	h := NewHandler(svc)
	r := gin.New()
	rg := r.Group("/api/v1")
	h.RegisterRoutes(rg)
	return r
}

// TestGetByID_ScheduledDate_SerializesAsDateOnly is the JSON-body-literal
// regression guard for the date-serialization bug (docs/api-contract.md
// Catatan Desain: "Kenapa field bertipe tanggal sekarang serialize sebagai
// YYYY-MM-DD..."): jobs.scheduled_date must appear as "YYYY-MM-DD" in the
// ACTUAL marshaled HTTP response body - a Go-level *dateonly.Date/*time.Time
// assertion (like every other test in this package) can't catch this class
// of bug at all, since it never inspects the JSON bytes.
func TestGetByID_ScheduledDate_SerializesAsDateOnly(t *testing.T) {
	repo := newFakeRepository()
	router := newTestRouter(repo)

	scheduled := dateonly.New(2026, time.August, 15)
	created, err := repo.Create(context.Background(), Job{
		CustomerID: uuid.New(), Title: "Servis rutin", ScheduledDate: &scheduled,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+created.ID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"scheduled_date":"2026-08-15"`, "must be literal YYYY-MM-DD in the response body")
	assert.NotContains(t, body, "T00:00:00Z", "must NOT be RFC3339 - this is exactly the regression this test guards against")
}

// TestGetByID_CompletedDate_SerializesAsDateOnly: same guard for
// completed_date (set automatically by UpdateStatus, not by client input -
// see service.go).
func TestGetByID_CompletedDate_SerializesAsDateOnly(t *testing.T) {
	repo := newFakeRepository()
	router := newTestRouter(repo)
	svc := newTestService(repo)
	created, err := svc.Create(context.Background(), CreateInput{CustomerID: uuid.New(), Title: "Servis"})
	require.NoError(t, err)
	_, err = svc.UpdateStatus(context.Background(), created.ID, StatusCompleted, uuid.New(), nil)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+created.ID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Regexp(t, `"completed_date":"\d{4}-\d{2}-\d{2}"`, body, "must be literal YYYY-MM-DD in the response body")
	assert.NotContains(t, body, "T00:00:00Z", "must NOT be RFC3339 - this is exactly the regression this test guards against")
}
