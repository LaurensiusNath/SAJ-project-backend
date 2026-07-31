package auth

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/nathan/cnc-pm-backend/internal/httpresponse"
	"github.com/nathan/cnc-pm-backend/internal/user"
)

// cookieName adalah nama cookie httpOnly yang menyimpan JWT access token -
// dibagi oleh Login (set cookie), Logout (hapus cookie), dan
// middleware.go/RequireAuth (baca cookie ini duluan sebelum fallback ke
// header Authorization). Lihat docs/api-contract.md#0 untuk kontraknya.
const cookieName = "access_token"

type Handler struct {
	svc *Service
	// secureCookie = true kalau APP_ENV=production (lihat cmd/api/main.go) -
	// menentukan flag Secure di cookie access_token. Di-inject dari luar,
	// bukan baca env sendiri, supaya package ini tetap tidak tahu-menahu
	// soal config (pola yang sama dengan Service.NewService/JWTSecret).
	secureCookie bool
}

func NewHandler(svc *Service, secureCookie bool) *Handler {
	return &Handler{svc: svc, secureCookie: secureCookie}
}

// RegisterRoutes: /auth/login publik (tidak lewat RequireAuth), tapi
// /auth/logout butuh auth valid - middleware-nya dipasang langsung di sini
// (bukan lewat rg.Use()) supaya tidak bergantung pada apakah rg yang di-pass
// main.go sudah punya RequireAuth atau belum (pola sama dengan
// settings.Handler.RegisterRoutes yang menyuntik requireAdmin per-route).
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/auth/login", h.Login)
	rg.POST("/auth/logout", h.svc.RequireAuth(), h.Logout)
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type userResponse struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Role  user.Role `json:"role"`
}

func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	token, u, err := h.svc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			httpresponse.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", err.Error())
			return
		}
		httpresponse.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}

	h.setAccessTokenCookie(c, token, AccessTokenTTLSeconds())
	httpresponse.Success(c, http.StatusOK, gin.H{"user": userResponse{
		ID: u.ID, Name: u.Name, Email: u.Email, Role: u.Role,
	}})
}

func (h *Handler) Logout(c *gin.Context) {
	// Perlu -1 di sini, BUKAN 0 - net/http.Cookie punya konvensi ganjil:
	// MaxAge: 0 berarti "jangan tulis atribut Max-Age sama sekali" (cookie
	// jadi session cookie biasa, TIDAK dihapus), sedangkan MaxAge negatif
	// yang benar-benar menghasilkan "Max-Age=0" tertulis di header
	// Set-Cookie - itu instruksi standar yang bikin browser menghapus
	// cookie sekarang juga. Ketahuan lewat verifikasi manual terhadap
	// server nyata (curl -i menunjukkan atribut Max-Age hilang total saat
	// masih pakai 0), bukan cuma dari baca dokumentasi.
	h.setAccessTokenCookie(c, "", -1)
	httpresponse.Success(c, http.StatusOK, gin.H{})
}

// setAccessTokenCookie memusatkan atribut cookie (SameSite=Lax, Secure
// hanya di production, HttpOnly, Path=/) di satu tempat - Login dan Logout
// HARUS memakai atribut yang identik, kalau tidak browser bisa menyimpan
// dua cookie access_token berbeda (path/attribute berbeda dianggap cookie
// berbeda) alih-alih menimpa/menghapus yang lama.
func (h *Handler) setAccessTokenCookie(c *gin.Context, token string, maxAgeSeconds int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(cookieName, token, maxAgeSeconds, "/", "", h.secureCookie, true)
}
