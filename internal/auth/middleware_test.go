package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nathan/cnc-pm-backend/internal/user"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// signTestToken bikin token JWT langsung (tanpa lewat Service.Login) supaya
// test middleware bisa mengontrol persis isi claims-nya (termasuk claims
// yang sengaja kedaluwarsa, untuk TestRequireAuth_ExpiredToken).
func signTestToken(t *testing.T, secret string, claims Claims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return signed
}

func newTestRouter(svc *Service, extra ...gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	handlers := append([]gin.HandlerFunc{svc.RequireAuth()}, extra...)
	handlers = append(handlers, func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/protected", handlers...)
	return r
}

func TestRequireAuth_MissingHeader(t *testing.T) {
	svc := NewService(newFakeUserRepository(), "test-secret")
	router := newTestRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_InvalidToken(t *testing.T) {
	svc := NewService(newFakeUserRepository(), "test-secret")
	router := newTestRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_ExpiredToken(t *testing.T) {
	svc := NewService(newFakeUserRepository(), "test-secret")
	router := newTestRouter(svc)
	expired := signTestToken(t, "test-secret", Claims{
		UserID: uuid.New(),
		Role:   user.RoleAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+expired)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_WrongSecretRejected(t *testing.T) {
	svc := NewService(newFakeUserRepository(), "test-secret")
	router := newTestRouter(svc)
	tokenSignedWithDifferentSecret := signTestToken(t, "attacker-secret", Claims{
		UserID: uuid.New(),
		Role:   user.RoleOwner,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenSignedWithDifferentSecret)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code, "a token signed with a different secret must be rejected")
}

func TestRequireAuth_ValidToken(t *testing.T) {
	svc := NewService(newFakeUserRepository(), "test-secret")
	router := newTestRouter(svc)
	valid := signTestToken(t, "test-secret", Claims{
		UserID: uuid.New(),
		Role:   user.RoleTeknisi,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+valid)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireRole(t *testing.T) {
	svc := NewService(newFakeUserRepository(), "test-secret")

	testCases := []struct {
		name       string
		role       user.Role
		wantStatus int
	}{
		{name: "owner is allowed", role: user.RoleOwner, wantStatus: http.StatusOK},
		{name: "admin is allowed", role: user.RoleAdmin, wantStatus: http.StatusOK},
		{name: "teknisi is forbidden", role: user.RoleTeknisi, wantStatus: http.StatusForbidden},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			router := newTestRouter(svc, RequireRole(user.RoleOwner, user.RoleAdmin))
			token := signTestToken(t, "test-secret", Claims{
				UserID: uuid.New(),
				Role:   tc.role,
				RegisteredClaims: jwt.RegisteredClaims{
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
				},
			})

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tc.wantStatus, w.Code)
		})
	}
}
