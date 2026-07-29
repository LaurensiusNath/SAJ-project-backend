package customer

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/nathan/cnc-pm-backend/internal/httpresponse"
)

type MachineHandler struct {
	svc *MachineService
}

func NewMachineHandler(svc *MachineService) *MachineHandler {
	return &MachineHandler{svc: svc}
}

// RegisterRoutes pakai ":id" (bukan ":customer_id") - gin/httprouter tidak
// mengizinkan nama wildcard berbeda di node path yang sama ("/customers/:id"
// dari customer.Handler vs "/customers/:customer_id" di sini akan panic
// saat startup: "conflicts with existing wildcard"). Jadi walau secara
// semantik ini "customer_id", parameternya tetap dibaca lewat c.Param("id").
func (h *MachineHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/customers/:id/machines", h.Create)
	rg.GET("/customers/:id/machines", h.List)
}

type createMachineRequest struct {
	MachineName  string  `json:"machine_name" binding:"required"`
	MachineType  *string `json:"machine_type"`
	SerialNumber *string `json:"serial_number"`
	Notes        *string `json:"notes"`
}

func (h *MachineHandler) Create(c *gin.Context) {
	customerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid customer id")
		return
	}

	var req createMachineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	created, err := h.svc.Create(c.Request.Context(), CreateMachineInput{
		CustomerID:   customerID,
		MachineName:  req.MachineName,
		MachineType:  req.MachineType,
		SerialNumber: req.SerialNumber,
		Notes:        req.Notes,
	})
	if err != nil {
		h.respondError(c, err)
		return
	}
	httpresponse.Success(c, http.StatusCreated, created)
}

func (h *MachineHandler) List(c *gin.Context) {
	customerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid customer id")
		return
	}

	machines, err := h.svc.ListByCustomer(c.Request.Context(), customerID)
	if err != nil {
		h.respondError(c, err)
		return
	}
	httpresponse.Success(c, http.StatusOK, machines)
}

func (h *MachineHandler) respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpresponse.Error(c, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, ErrInvalidMachineName):
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		httpresponse.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}
