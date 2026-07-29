package settings

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/nathan/cnc-pm-backend/internal/httpresponse"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes menerima requireAdmin terpisah dari rg - GET dibuka untuk
// SEMUA user yang sudah login (cukup lolos RequireAuth yang dipasang di rg
// oleh main.go), tapi PUT dibatasi role owner/admin sesuai api-contract.md.
// Package ini sengaja tidak import internal/auth secara langsung (supaya
// tidak ada modul bisnis yang bergantung ke auth) - middleware role-nya
// disuntik dari main.go sebagai gin.HandlerFunc biasa.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, requireAdmin gin.HandlerFunc) {
	rg.GET("/settings/company", h.Get)
	rg.PUT("/settings/company", requireAdmin, h.Update)
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
