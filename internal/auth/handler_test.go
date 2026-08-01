package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nathan/cnc-pm-backend/internal/user"
)

func newTestRouterWithHandler(secureCookie bool) (*gin.Engine, *fakeUserRepository) {
	repo := newFakeUserRepository()
	svc := NewService(repo, "test-secret")
	h := NewHandler(svc, secureCookie)
	r := gin.New()
	rg := r.Group("/api/v1")
	h.RegisterRoutes(rg)
	return r, repo
}

// TestLogin_SetsHttpOnlyCookie_NoTokenInBody proves the core contract change
// (docs/api-contract.md#0): the raw JWT must be delivered via an HttpOnly
// cookie, never in the JSON body - returning it in both places would defeat
// the point of HttpOnly (JS could just read it from the response instead).
func TestLogin_SetsHttpOnlyCookie_NoTokenInBody(t *testing.T) {
	router, repo := newTestRouterWithHandler(false)
	seeded := seedUser(t, repo, "owner@cncservis.local", "ChangeMe123!", user.RoleOwner)

	body, _ := json.Marshal(map[string]string{"email": "owner@cncservis.local", "password": "ChangeMe123!"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "access_token", "raw token must never appear in the JSON body")

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	cookie := cookies[0]
	assert.Equal(t, cookieName, cookie.Name)
	assert.NotEmpty(t, cookie.Value)
	assert.True(t, cookie.HttpOnly)
	assert.False(t, cookie.Secure, "Secure must be false when secureCookie=false (non-production)")
	assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
	assert.Equal(t, "/", cookie.Path)
	assert.Equal(t, AccessTokenTTLSeconds(), cookie.MaxAge)

	var decoded struct {
		Success bool `json:"success"`
		Data    struct {
			User struct {
				ID    string `json:"id"`
				Name  string `json:"name"`
				Email string `json:"email"`
				Role  string `json:"role"`
			} `json:"user"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &decoded))
	assert.Equal(t, seeded.ID.String(), decoded.Data.User.ID)
	assert.Equal(t, seeded.Email, decoded.Data.User.Email)
	assert.Equal(t, string(user.RoleOwner), decoded.Data.User.Role)
}

// TestLogin_SecureCookie_WhenConfiguredForProduction proves Secure is only
// set when the handler is configured for production (APP_ENV=production in
// cmd/api/main.go) - Secure cookies are silently dropped by browsers over
// plain HTTP, which would break local development if always-on.
func TestLogin_SecureCookie_WhenConfiguredForProduction(t *testing.T) {
	router, repo := newTestRouterWithHandler(true)
	seedUser(t, repo, "owner@cncservis.local", "ChangeMe123!", user.RoleOwner)

	body, _ := json.Marshal(map[string]string{"email": "owner@cncservis.local", "password": "ChangeMe123!"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.True(t, cookies[0].Secure)
}

func TestLogin_WrongPassword_NoCookieSet(t *testing.T) {
	router, repo := newTestRouterWithHandler(false)
	seedUser(t, repo, "owner@cncservis.local", "ChangeMe123!", user.RoleOwner)

	body, _ := json.Marshal(map[string]string{"email": "owner@cncservis.local", "password": "wrong"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Empty(t, w.Result().Cookies())
}

// TestLogout_RequiresAuth proves /auth/logout is NOT public - unlike
// /auth/login, it must reject requests with no valid token.
func TestLogout_RequiresAuth(t *testing.T) {
	router, _ := newTestRouterWithHandler(false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestLogout_ClearsCookie proves logout sends Max-Age=0 for access_token,
// the standard browser signal to delete the cookie immediately.
func TestLogout_ClearsCookie(t *testing.T) {
	router, repo := newTestRouterWithHandler(false)
	seedUser(t, repo, "owner@cncservis.local", "ChangeMe123!", user.RoleOwner)

	svc := NewService(repo, "test-secret")
	token, _, err := svc.Login(context.Background(), "owner@cncservis.local", "ChangeMe123!")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, cookieName, cookies[0].Name)
	// -1 di sini bukan salah ketik: net/http.Cookie.MaxAge==0 berarti "jangan
	// tulis atribut Max-Age sama sekali" (session cookie, TIDAK dihapus).
	// Untuk benar-benar menghasilkan "Max-Age=0" di wire (yang memerintahkan
	// browser menghapus cookie), Cookie.MaxAge harus negatif saat DITULIS -
	// begitu di-parse balik dari header respons, http.Response.Cookies()
	// menyimpannya lagi sebagai -1. Diverifikasi lewat curl -i terhadap
	// server nyata (lihat handler.go/Logout), bukan cuma dugaan dari dokumentasi.
	assert.Equal(t, -1, cookies[0].MaxAge)
	assert.Empty(t, cookies[0].Value)
}

// TestMe_RequiresAuth proves GET /auth/me is NOT public - same requirement
// as /auth/logout.
func TestMe_RequiresAuth(t *testing.T) {
	router, _ := newTestRouterWithHandler(false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestMe_ReturnsSameShapeAsLogin proves the core contract requirement: the
// body of GET /auth/me must be byte-for-byte identical in SHAPE to POST
// /auth/login's body, so the frontend can reuse one TypeScript type for both.
func TestMe_ReturnsSameShapeAsLogin(t *testing.T) {
	router, repo := newTestRouterWithHandler(false)
	seeded := seedUser(t, repo, "teknisi@cncservis.local", "ChangeMe123!", user.RoleTeknisi)

	svc := NewService(repo, "test-secret")
	token, _, err := svc.Login(context.Background(), "teknisi@cncservis.local", "ChangeMe123!")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var decoded struct {
		Success bool `json:"success"`
		Data    struct {
			User struct {
				ID    string `json:"id"`
				Name  string `json:"name"`
				Email string `json:"email"`
				Role  string `json:"role"`
			} `json:"user"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &decoded))
	assert.Equal(t, seeded.ID.String(), decoded.Data.User.ID)
	assert.Equal(t, seeded.Name, decoded.Data.User.Name)
	assert.Equal(t, seeded.Email, decoded.Data.User.Email)
	assert.Equal(t, string(user.RoleTeknisi), decoded.Data.User.Role)
}

// TestMe_ViaBearerHeader proves the fallback path also works for /auth/me,
// not just the cookie path.
func TestMe_ViaBearerHeader(t *testing.T) {
	router, repo := newTestRouterWithHandler(false)
	seedUser(t, repo, "owner@cncservis.local", "ChangeMe123!", user.RoleOwner)

	svc := NewService(repo, "test-secret")
	token, _, err := svc.Login(context.Background(), "owner@cncservis.local", "ChangeMe123!")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
