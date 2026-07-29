package customer

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/nathan/cnc-pm-backend/internal/httpresponse"
)

// Handler menjembatani HTTP (Gin) ke Service. Tugasnya cuma tiga: parse +
// validasi bentuk request, panggil Service, format response - tidak ada
// business logic di sini (itu tugas service.go).
type Handler struct {
	svc        *Service
	machineSvc *MachineService
}

// NewHandler butuh MachineService juga - GetByID mengembalikan customer
// beserta nested machines-nya (lihat api-contract.md: "Detail customer +
// nested machines"), jadi handler ini perlu menggabungkan dua service.
func NewHandler(svc *Service, machineSvc *MachineService) *Handler {
	return &Handler{svc: svc, machineSvc: machineSvc}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/customers", h.Create)
	rg.GET("/customers", h.List)
	rg.GET("/customers/:id", h.GetByID)
	rg.PUT("/customers/:id", h.Update)
	rg.DELETE("/customers/:id", h.Delete)
}

type createCustomerRequest struct {
	Name         string  `json:"name" binding:"required"`
	CustomerType string  `json:"customer_type" binding:"required,oneof=badan_usaha perorangan"`
	Phone        *string `json:"phone"`
	Email        *string `json:"email"`
	Address      *string `json:"address"`
	CompanyName  *string `json:"company_name"`
}

func (h *Handler) Create(c *gin.Context) {
	var req createCustomerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	created, err := h.svc.Create(c.Request.Context(), CreateInput{
		Name:         req.Name,
		CustomerType: CustomerType(req.CustomerType),
		Phone:        req.Phone,
		Email:        req.Email,
		Address:      req.Address,
		CompanyName:  req.CompanyName,
	})
	if err != nil {
		h.respondError(c, err)
		return
	}
	httpresponse.Success(c, http.StatusCreated, created)
}

// customerDetailResponse membungkus Customer + nested Machines - beda dari
// Customer biasa (dipakai Create/List/Update) yang tidak menyertakan
// machines sama sekali, sesuai api-contract.md yang cuma menyebut nested
// machines untuk GET /customers/{id} secara spesifik.
type customerDetailResponse struct {
	Customer
	Machines []Machine `json:"machines"`
}

func (h *Handler) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid customer id")
		return
	}

	found, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		h.respondError(c, err)
		return
	}

	machines, err := h.machineSvc.ListByCustomer(c.Request.Context(), id)
	if err != nil {
		h.respondError(c, err)
		return
	}

	httpresponse.Success(c, http.StatusOK, customerDetailResponse{Customer: found, Machines: machines})
}

func (h *Handler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	var search *string
	if s := c.Query("search"); s != "" {
		search = &s
	}
	var customerType *CustomerType
	if ct := c.Query("customer_type"); ct != "" {
		parsed := CustomerType(ct)
		customerType = &parsed
	}

	result, err := h.svc.List(c.Request.Context(), ListParams{
		Search:       search,
		CustomerType: customerType,
		Page:         int32(page),
		Limit:        int32(limit),
	})
	if err != nil {
		h.respondError(c, err)
		return
	}
	httpresponse.SuccessWithMeta(c, http.StatusOK, result.Customers, gin.H{
		"page":  page,
		"total": result.Total,
	})
}

type updateCustomerRequest struct {
	Name        string  `json:"name" binding:"required"`
	Phone       *string `json:"phone"`
	Email       *string `json:"email"`
	Address     *string `json:"address"`
	CompanyName *string `json:"company_name"`
}

func (h *Handler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid customer id")
		return
	}

	var req updateCustomerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	updated, err := h.svc.Update(c.Request.Context(), UpdateInput{
		ID:          id,
		Name:        req.Name,
		Phone:       req.Phone,
		Email:       req.Email,
		Address:     req.Address,
		CompanyName: req.CompanyName,
	})
	if err != nil {
		h.respondError(c, err)
		return
	}
	httpresponse.Success(c, http.StatusOK, updated)
}

func (h *Handler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid customer id")
		return
	}

	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		h.respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpresponse.Error(c, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, ErrInvalidName), errors.Is(err, ErrInvalidType):
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		httpresponse.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}
