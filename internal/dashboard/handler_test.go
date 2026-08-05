package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nathan/cnc-pm-backend/internal/dateonly"
)

// TestGetSummary_Period_SerializesAsDateOnly is the JSON-body-literal
// regression guard for the date-serialization bug (docs/api-contract.md
// Catatan Desain: "Kenapa field bertipe tanggal sekarang serialize sebagai
// YYYY-MM-DD..."): financial.period.from/to must appear as "YYYY-MM-DD" in
// the ACTUAL marshaled HTTP response body - a Go-level dateonly.Date
// assertion (like the tests in service_test.go) can't catch this class of
// bug, since it never inspects the JSON bytes.
func TestGetSummary_Period_SerializesAsDateOnly(t *testing.T) {
	repo := &fakeRepository{summary: Summary{
		Financial: FinancialSummary{
			Period: Period{
				From: dateonly.New(2026, time.August, 1),
				To:   dateonly.New(2026, time.August, 31),
			},
		},
	}}
	svc := NewService(repo)
	h := NewHandler(svc)
	r := gin.New()
	rg := r.Group("/api/v1")
	h.RegisterRoutes(rg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/summary", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"from":"2026-08-01"`, "must be literal YYYY-MM-DD in the response body")
	assert.Contains(t, body, `"to":"2026-08-31"`, "must be literal YYYY-MM-DD in the response body")
	assert.NotContains(t, body, "T00:00:00Z", "must NOT be RFC3339 - this is exactly the regression this test guards against")
}

// TestGetSummary_UpcomingScheduledDate_SerializesAsDateOnly: same guard for
// upcoming_7_days[].scheduled_date - a separate query path from job/invoice
// (see db/queries/dashboard.sql), but the same underlying bug class.
func TestGetSummary_UpcomingScheduledDate_SerializesAsDateOnly(t *testing.T) {
	scheduled := dateonly.New(2026, time.August, 6)
	repo := &fakeRepository{summary: Summary{
		Jobs: JobsSummary{
			Upcoming7Days: []ScheduledJobRef{
				{JobCode: "JOB-2026-0001", CustomerName: "Bengkel A", ScheduledDate: &scheduled},
			},
		},
	}}
	svc := NewService(repo)
	h := NewHandler(svc)
	r := gin.New()
	rg := r.Group("/api/v1")
	h.RegisterRoutes(rg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/summary", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"scheduled_date":"2026-08-06"`, "must be literal YYYY-MM-DD in the response body")
	assert.NotContains(t, body, "T00:00:00Z", "must NOT be RFC3339 - this is exactly the regression this test guards against")
}
