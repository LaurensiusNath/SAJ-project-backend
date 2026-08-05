package taxreport

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nathan/cnc-pm-backend/internal/httpresponse"
)

const dateLayout = "2006-01-02"

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes TIDAK memasang pembatasan role di sini - owner/admin-only
// dilakukan lewat pemilihan route group di cmd/api/main.go
// (taxreportHandler.RegisterRoutes(adminGroup), grup yang sama dipakai
// user.Handler dan dashboard.Handler) - modul bisnis tidak menegakkan
// kebijakan otorisasi sendiri.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/reports/tax-summary", h.GetSummary)
}

func (h *Handler) GetSummary(c *gin.Context) {
	from, err := parseOptionalDate(c.Query("period_from"))
	if err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "period_from must be in YYYY-MM-DD format")
		return
	}
	to, err := parseOptionalDate(c.Query("period_to"))
	if err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "period_to must be in YYYY-MM-DD format")
		return
	}

	summary, err := h.svc.GetSummary(c.Request.Context(), GetSummaryParams{From: from, To: to})
	if err != nil {
		h.respondError(c, err)
		return
	}
	httpresponse.Success(c, http.StatusOK, summary)
}

// parseOptionalDate: duplikasi ke-3 dari fungsi identik di job/invoice/
// dashboard handler.go (lihat Technical Debt item #10 di
// docs/api-contract.md) - query param di sini juga string mentah lewat
// c.Query(), sama seperti dashboard, jadi tidak bisa lewat UnmarshalJSON
// otomatis seperti request body JSON.
func parseOptionalDate(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (h *Handler) respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidPeriod):
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		httpresponse.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}
