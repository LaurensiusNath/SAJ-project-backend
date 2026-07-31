package job

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/nathan/cnc-pm-backend/internal/auth"
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
	rg.POST("/jobs", h.Create)
	rg.GET("/jobs", h.List)
	rg.GET("/jobs/:id", h.GetByID)
	rg.PATCH("/jobs/:id/status", h.UpdateStatus)
	rg.PATCH("/jobs/:id/assign", h.AssignTechnician)
}

type createJobRequest struct {
	CustomerID    uuid.UUID  `json:"customer_id" binding:"required"`
	MachineID     *uuid.UUID `json:"machine_id"`
	Title         string     `json:"title" binding:"required"`
	Description   *string    `json:"description"`
	ScheduledDate *string    `json:"scheduled_date"`
}

func (h *Handler) Create(c *gin.Context) {
	var req createJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	scheduledDate, err := parseOptionalDate(req.ScheduledDate)
	if err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "scheduled_date must be in YYYY-MM-DD format")
		return
	}

	created, err := h.svc.Create(c.Request.Context(), CreateInput{
		CustomerID:    req.CustomerID,
		MachineID:     req.MachineID,
		Title:         req.Title,
		Description:   req.Description,
		ScheduledDate: scheduledDate,
	})
	if err != nil {
		h.respondError(c, err)
		return
	}
	httpresponse.Success(c, http.StatusCreated, created)
}

// GetByID mengembalikan Job + status_history + costs (lihat
// docs/api-contract.md: "wajib nested, ini requirement, bukan opsional") -
// beda dari List yang tetap mengembalikan Job polos.
func (h *Handler) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid job id")
		return
	}

	found, err := h.svc.GetDetail(c.Request.Context(), id)
	if err != nil {
		h.respondError(c, err)
		return
	}
	httpresponse.Success(c, http.StatusOK, found)
}

func (h *Handler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	var status *JobStatus
	if s := c.Query("status"); s != "" {
		parsed := JobStatus(s)
		status = &parsed
	}

	var customerID *uuid.UUID
	if cid := c.Query("customer_id"); cid != "" {
		parsed, err := uuid.Parse(cid)
		if err != nil {
			httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid customer_id")
			return
		}
		customerID = &parsed
	}

	result, err := h.svc.List(c.Request.Context(), ListParams{
		Status:     status,
		CustomerID: customerID,
		Page:       int32(page),
		Limit:      int32(limit),
	})
	if err != nil {
		h.respondError(c, err)
		return
	}
	httpresponse.SuccessWithMeta(c, http.StatusOK, result.Jobs, gin.H{
		"page":  page,
		"total": result.Total,
	})
}

type updateStatusRequest struct {
	Status string  `json:"status" binding:"required,oneof=requested scheduled in_progress completed cancelled"`
	Notes  *string `json:"notes"`
}

func (h *Handler) UpdateStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid job id")
		return
	}

	var req updateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	// changedBy WAJIB dari JWT claim (siapa yang login), bukan dari body -
	// route ini selalu di belakang RequireAuth (lihat main.go), jadi selalu
	// ada; kalaupun tidak ada (harusnya mustahil), lebih aman gagal 401
	// daripada diam-diam catat UUID kosong sebagai "siapa yang mengubah".
	changedBy, ok := auth.UserIDFromContext(c)
	if !ok {
		httpresponse.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing authentication")
		return
	}

	updated, err := h.svc.UpdateStatus(c.Request.Context(), id, JobStatus(req.Status), changedBy, req.Notes)
	if err != nil {
		h.respondError(c, err)
		return
	}
	httpresponse.Success(c, http.StatusOK, updated)
}

// assignTechnicianRequest.ExpectedUpdatedAt wajib diisi client dengan
// updated_at persis yang terakhir dia baca (dari GET /jobs/{id} sebelumnya) -
// ini kontrak optimistic locking, lihat internal/job/repository.go/AssignTechnician
// untuk alasannya. time.Time otomatis menerima format RFC3339 (format yang
// sama dipakai json.Marshal saat serialize field updated_at di response).
type assignTechnicianRequest struct {
	TechnicianID      uuid.UUID `json:"technician_id" binding:"required"`
	ExpectedUpdatedAt time.Time `json:"expected_updated_at" binding:"required"`
}

func (h *Handler) AssignTechnician(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid job id")
		return
	}

	var req assignTechnicianRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	updated, err := h.svc.AssignTechnician(c.Request.Context(), id, req.TechnicianID, req.ExpectedUpdatedAt)
	if err != nil {
		h.respondError(c, err)
		return
	}
	httpresponse.Success(c, http.StatusOK, updated)
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
	case errors.Is(err, ErrConflict):
		httpresponse.Error(c, http.StatusConflict, "CONFLICT", err.Error())
	case errors.Is(err, ErrInvalidTitle), errors.Is(err, ErrInvalidStatus), errors.Is(err, ErrInvalidCustomer), errors.Is(err, ErrInvalidReference):
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		httpresponse.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}
