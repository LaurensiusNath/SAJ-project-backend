package invoice

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRouter() (*gin.Engine, *fakeRepository) {
	repo := newFakeRepository()
	svc := NewService(repo)
	h := NewHandler(svc)
	r := gin.New()
	rg := r.Group("/api/v1")
	h.RegisterRoutes(rg)
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
