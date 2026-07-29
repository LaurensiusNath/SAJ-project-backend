package settings

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/nathan/cnc-pm-backend/internal/httpresponse"
)

// Handler belum menegakkan "hanya role owner/admin" yang disebut
// api-contract.md untuk PUT - project ini belum punya modul auth/middleware
// role sama sekali di modul manapun (Customer, Job juga belum). Ini gap
// lintas-modul, bukan sesuatu yang bisa diselesaikan cuma di sini.
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/settings/company", h.Get)
	rg.PUT("/settings/company", h.Update)
}

func (h *Handler) Get(c *gin.Context) {
	result, err := h.svc.Get(c.Request.Context())
	if err != nil {
		httpresponse.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	httpresponse.Success(c, http.StatusOK, result)
}

type updateRequest struct {
	CompanyName          string           `json:"company_name" binding:"required"`
	NPWP                 *string          `json:"npwp"`
	IsPKP                bool             `json:"is_pkp"`
	DefaultTaxPercentage *decimal.Decimal `json:"default_tax_percentage" binding:"required"`
	DefaultPPh23Rate     *decimal.Decimal `json:"default_pph23_rate" binding:"required"`
}

func (h *Handler) Update(c *gin.Context) {
	var req updateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	updated, err := h.svc.Update(c.Request.Context(), UpdateInput{
		CompanyName:          req.CompanyName,
		NPWP:                 req.NPWP,
		IsPKP:                req.IsPKP,
		DefaultTaxPercentage: *req.DefaultTaxPercentage,
		DefaultPPh23Rate:     *req.DefaultPPh23Rate,
	})
	if err != nil {
		h.respondError(c, err)
		return
	}
	httpresponse.Success(c, http.StatusOK, updated)
}

func (h *Handler) respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidCompanyName), errors.Is(err, ErrInvalidPercentage):
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		httpresponse.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}
