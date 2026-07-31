package invoice

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/nathan/cnc-pm-backend/internal/httpresponse"
)

const dateLayout = "2006-01-02"

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/jobs/:id/invoice", h.CreateFromJob)
	rg.GET("/invoices", h.List)
	rg.GET("/invoices/:id", h.GetByID)
	rg.PATCH("/invoices/:id/faktur-pajak", h.UpdateFakturPajak)
	rg.PATCH("/invoices/:id/status", h.UpdateStatus)
	rg.POST("/invoices/:id/payments", h.RecordPayment)
	rg.GET("/invoices/:id/payments", h.ListPayments)
}

type createInvoiceRequest struct {
	TaxPercentage *decimal.Decimal `json:"tax_percentage"`
	DueDate       *string          `json:"due_date"`
}

func (h *Handler) CreateFromJob(c *gin.Context) {
	jobID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid job id")
		return
	}

	var req createInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	dueDate, err := parseOptionalDate(req.DueDate)
	if err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "due_date must be in YYYY-MM-DD format")
		return
	}

	created, err := h.svc.CreateFromJob(c.Request.Context(), jobID, CreateInput{
		TaxPercentage: req.TaxPercentage,
		DueDate:       dueDate,
	})
	if err != nil {
		h.respondError(c, err)
		return
	}
	httpresponse.Success(c, http.StatusCreated, created)
}

func (h *Handler) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid invoice id")
		return
	}

	found, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		h.respondError(c, err)
		return
	}
	httpresponse.Success(c, http.StatusOK, found)
}

func (h *Handler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	var status *Status
	if s := c.Query("status"); s != "" {
		parsed := Status(s)
		status = &parsed
	}

	result, err := h.svc.List(c.Request.Context(), ListParams{
		Status: status,
		Page:   int32(page),
		Limit:  int32(limit),
	})
	if err != nil {
		h.respondError(c, err)
		return
	}
	httpresponse.SuccessWithMeta(c, http.StatusOK, result.Invoices, gin.H{
		"page":  page,
		"total": result.Total,
	})
}

type updateFakturPajakRequest struct {
	NomorFakturPajak string `json:"nomor_faktur_pajak" binding:"required"`
}

func (h *Handler) UpdateFakturPajak(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid invoice id")
		return
	}

	var req updateFakturPajakRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	updated, err := h.svc.UpdateFakturPajak(c.Request.Context(), id, req.NomorFakturPajak)
	if err != nil {
		h.respondError(c, err)
		return
	}
	httpresponse.Success(c, http.StatusOK, updated)
}

type updateStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=draft sent paid overdue cancelled"`
}

func (h *Handler) UpdateStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid invoice id")
		return
	}

	var req updateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	updated, err := h.svc.UpdateStatus(c.Request.Context(), id, Status(req.Status))
	if err != nil {
		h.respondError(c, err)
		return
	}
	httpresponse.Success(c, http.StatusOK, updated)
}

type recordPaymentRequest struct {
	Amount              decimal.Decimal `json:"amount" binding:"required"`
	PaymentMethod       string          `json:"payment_method" binding:"required,oneof=transfer cash other"`
	BuktiPotongPPh23Ref *string         `json:"bukti_potong_pph23_ref"`
	Notes               *string         `json:"notes"`
}

func (h *Handler) RecordPayment(c *gin.Context) {
	invoiceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid invoice id")
		return
	}

	var req recordPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	created, err := h.svc.RecordPayment(c.Request.Context(), invoiceID, CreatePaymentInput{
		Amount:              req.Amount,
		PaymentMethod:       PaymentMethod(req.PaymentMethod),
		BuktiPotongPPh23Ref: req.BuktiPotongPPh23Ref,
		Notes:               req.Notes,
	})
	if err != nil {
		h.respondError(c, err)
		return
	}
	httpresponse.Success(c, http.StatusCreated, created)
}

func (h *Handler) ListPayments(c *gin.Context) {
	invoiceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid invoice id")
		return
	}

	payments, err := h.svc.ListPayments(c.Request.Context(), invoiceID)
	if err != nil {
		h.respondError(c, err)
		return
	}
	httpresponse.Success(c, http.StatusOK, payments)
}

func parseOptionalDate(s *string) (*time.Time, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	t, err := time.Parse(dateLayout, *s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (h *Handler) respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpresponse.Error(c, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, ErrJobNotCompleted),
		errors.Is(err, ErrAlreadyInvoiced),
		errors.Is(err, ErrInvalidTaxPercentage),
		errors.Is(err, ErrInvalidStatus),
		errors.Is(err, ErrInvalidFakturPajak),
		errors.Is(err, ErrInvalidAmount),
		errors.Is(err, ErrInvalidPaymentMethod),
		errors.Is(err, ErrInvoiceNotPayable):
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		httpresponse.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}
