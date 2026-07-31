package auth

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/nathan/cnc-pm-backend/internal/httpresponse"
	"github.com/nathan/cnc-pm-backend/internal/user"
)

const (
	contextKeyUserID = "auth_user_id"
	contextKeyRole   = "auth_role"
)

// RequireAuth mengambil token JWT dari cookie access_token DULU (jalur utama
// untuk frontend Next.js - lihat docs/api-contract.md#0), baru fallback ke
// header "Authorization: Bearer <token>" kalau cookie-nya tidak ada (tetap
// didukung untuk testing manual/Postman, tooling, client non-browser).
// Setelah token didapat, validasi JWT (tanda tangan + kedaluwarsa) sama
// persis seperti sebelumnya, lalu taruh user_id & role hasil decode ke
// gin.Context - handler/middleware di belakangnya (termasuk RequireRole)
// tinggal baca dari context, tidak decode ulang tokennya.
//
// Method receiver ("s *Service"), bukan fungsi package-level biasa, karena
// butuh akses ke s.secret untuk validasi tanda tangan.
func (s *Service) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, ok := tokenFromRequest(c)
		if !ok {
			httpresponse.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid access token")
			c.Abort()
			return
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
			// Guard eksplisit terhadap "algorithm confusion attack" - tanpa
			// ini, penyerang bisa kirim token dengan header alg berbeda
			// (mis. "none", atau RS256 memakai public key sebagai HMAC
			// secret) supaya validasi tanda tangan diam-diam terlewati.
			// Cuma terima keluarga algoritma HMAC yang memang kita pakai
			// untuk menandatangani (SigningMethodHS256 di service.go).
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return s.secret, nil
		})
		if err != nil || !token.Valid {
			httpresponse.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or expired token")
			c.Abort()
			return
		}

		c.Set(contextKeyUserID, claims.UserID)
		c.Set(contextKeyRole, claims.Role)
		c.Next()
	}
}

// tokenFromRequest mengembalikan token mentah dari cookie access_token kalau
// ada (dan tidak kosong - Logout menyetel cookie ini ke string kosong saat
// menghapusnya, itu bukan token valid), atau fallback ke header
// "Authorization: Bearer <token>". Cookie diprioritaskan sesuai kontrak.
func tokenFromRequest(c *gin.Context) (string, bool) {
	if cookieValue, err := c.Cookie(cookieName); err == nil && cookieValue != "" {
		return cookieValue, true
	}

	header := c.GetHeader("Authorization")
	tokenString, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || tokenString == "" {
		return "", false
	}
	return tokenString, true
}

// RequireRole HARUS dipasang setelah RequireAuth di rantai middleware yang
// sama (lihat cmd/api/main.go) - dia cuma membaca context yang sudah diisi
// RequireAuth, tidak pernah decode token sendiri. Fungsi package-level biasa
// (bukan method di Service) karena tidak butuh apapun dari Service - cukup
// baca context.
func RequireRole(roles ...user.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		value, ok := c.Get(contextKeyRole)
		if !ok {
			httpresponse.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing authentication")
			c.Abort()
			return
		}
		role := value.(user.Role)
		for _, allowed := range roles {
			if role == allowed {
				c.Next()
				return
			}
		}
		httpresponse.Error(c, http.StatusForbidden, "FORBIDDEN", "insufficient permissions for this action")
		c.Abort()
	}
}

// UserIDFromContext membaca user_id yang sudah divalidasi RequireAuth dari
// gin.Context - dipakai modul bisnis (mis. job.Handler) yang perlu tahu
// SIAPA yang melakukan sebuah aksi (mis. job_status_history.changed_by),
// bukan cuma "apakah request ini terautentikasi".
//
// Ini BUKAN pelanggaran terhadap prinsip "modul bisnis tidak boleh
// bergantung ke internal/auth" yang dipegang di settings/user - itu
// prinsip untuk menghindari modul bisnis MENEGAKKAN kebijakan otorisasi
// sendiri (duplikasi RequireRole). Fungsi ini cuma MEMBACA data yang sudah
// divalidasi & ditaruh middleware yang sudah dipasang di router - sama
// sifatnya dengan membaca header request biasa, bukan keputusan
// otorisasi baru.
func UserIDFromContext(c *gin.Context) (uuid.UUID, bool) {
	value, ok := c.Get(contextKeyUserID)
	if !ok {
		return uuid.UUID{}, false
	}
	id, ok := value.(uuid.UUID)
	return id, ok
}
