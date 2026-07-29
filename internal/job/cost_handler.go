package job

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/nathan/cnc-pm-backend/internal/httpresponse"
)

type CostHandler struct {
	svc *CostService
}

func NewCostHandler(svc *CostService) *CostHandler {
	return &CostHandler{svc: svc}
}

func (h *CostHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/jobs/:id/costs", h.Create)
	rg.GET("/jobs/:id/costs", h.List)
	rg.DELETE("/jobs/:id/costs/:cost_id", h.Delete)
}

type createCostRequest struct {
	CostType      string           `json:"cost_type" binding:"required,oneof=labor spare_part transport other"`
	Description   string           `json:"description" binding:"required"`
	Quantity      decimal.Decimal  `json:"quantity" binding:"required"`
	PurchasePrice *decimal.Decimal `json:"purchase_price"`
	SellingPrice  decimal.Decimal  `json:"selling_price" binding:"required"`
}

func (h *CostHandler) Create(c *gin.Context) {
	jobID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid job id")
		return
	}

	var req createCostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	created, err := h.svc.Create(c.Request.Context(), CreateCostInput{
		JobID:         jobID,
		CostType:      CostType(req.CostType),
		Description:   req.Description,
		Quantity:      req.Quantity,
		PurchasePrice: req.PurchasePrice,
		SellingPrice:  req.SellingPrice,
	})
	if err != nil {
		h.respondError(c, err)
		return
	}
	httpresponse.Success(c, http.StatusCreated, created)
}

// costResponse membungkus data + meta.total_selling/total_margin sesuai
// docs/api-contract.md - beda dari pola meta.{page,total} di Customer/Job
// (list ini tidak dipaginasi, satu job biasanya cuma punya beberapa baris
// biaya).
func (h *CostHandler) List(c *gin.Context) {
	jobID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid job id")
		return
	}

	result, err := h.svc.ListByJob(c.Request.Context(), jobID)
	if err != nil {
		h.respondError(c, err)
		return
	}
	httpresponse.SuccessWithMeta(c, http.StatusOK, result.Costs, gin.H{
		"total_selling": result.Totals.TotalSelling,
		"total_margin":  result.Totals.TotalMargin,
	})
}

func (h *CostHandler) Delete(c *gin.Context) {
	jobID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid job id")
		return
	}
	costID, err := uuid.Parse(c.Param("cost_id"))
	if err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid cost id")
		return
	}

	if err := h.svc.Delete(c.Request.Context(), jobID, costID); err != nil {
		h.respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *CostHandler) respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrCostNotFound):
		httpresponse.Error(c, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, ErrInvalidCostType),
		errors.Is(err, ErrInvalidQuantity),
		errors.Is(err, ErrInvalidSellingPrice),
		errors.Is(err, ErrPurchasePriceNotAllowed),
		errors.Is(err, ErrInvalidReference):
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		httpresponse.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}
