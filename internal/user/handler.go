package user

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/nathan/cnc-pm-backend/internal/httpresponse"
)

// Handler ini TIDAK menegakkan role sendiri - pembatasan "hanya owner/admin
// boleh POST /users" dipasang lewat middleware di level route group
// (cmd/api/main.go), bukan di sini, supaya package ini tidak perlu
// bergantung ke internal/auth (lihat catatan import satu arah di
// repository.go).
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/users", h.Create)
	rg.GET("/users", h.List)
}

type createUserRequest struct {
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
	Role     string `json:"role" binding:"required,oneof=owner admin teknisi"`
}

func (h *Handler) Create(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	created, err := h.svc.Create(c.Request.Context(), CreateInput{
		Name:     req.Name,
		Email:    req.Email,
		Password: req.Password,
		Role:     Role(req.Role),
	})
	if err != nil {
		h.respondError(c, err)
		return
	}
	httpresponse.Success(c, http.StatusCreated, created)
}

// List menangani GET /users?role=... - role di query param string biasa
// (bukan JSON body), jadi dikonversi ke Role dulu sebelum diteruskan ke
// Service (yang memvalidasi apakah nilainya salah satu role yang dikenal).
func (h *Handler) List(c *gin.Context) {
	var roleFilter *Role
	if raw := c.Query("role"); raw != "" {
		r := Role(raw)
		roleFilter = &r
	}

	users, err := h.svc.List(c.Request.Context(), roleFilter)
	if err != nil {
		h.respondError(c, err)
		return
	}
	httpresponse.Success(c, http.StatusOK, users)
}

func (h *Handler) respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrDuplicateEmail):
		httpresponse.Error(c, http.StatusConflict, "CONFLICT", err.Error())
	case errors.Is(err, ErrInvalidName), errors.Is(err, ErrInvalidEmail), errors.Is(err, ErrInvalidRole), errors.Is(err, ErrInvalidPassword):
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		httpresponse.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}
