package auth

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/nathan/cnc-pm-backend/internal/httpresponse"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes hanya POST /auth/login - satu-satunya endpoint publik
// (tidak lewat RequireAuth), sesuai api-contract.md: "Semua endpoint
// (kecuali /auth/login) butuh header Authorization: Bearer <access_token>".
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/auth/login", h.Login)
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	token, err := h.svc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			httpresponse.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", err.Error())
			return
		}
		httpresponse.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	httpresponse.Success(c, http.StatusOK, gin.H{"access_token": token})
}
