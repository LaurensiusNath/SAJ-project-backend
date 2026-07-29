package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/nathan/cnc-pm-backend/internal/user"
)

// ErrInvalidCredentials dipakai untuk DUA kasus berbeda (email tidak
// ditemukan, ATAU password salah) - sengaja tidak dibedakan. Membedakan
// keduanya ("email tidak terdaftar" vs "password salah") membocorkan
// informasi ke penyerang soal email mana yang valid di sistem (user
// enumeration) - respons harus identik untuk keduanya.
var ErrInvalidCredentials = errors.New("invalid email or password")

// accessTokenTTL: token berlaku 24 jam, tanpa refresh token - kontrak tidak
// menyebut refresh token sama sekali, dan untuk skala aplikasi internal satu
// bengkel, staf login ulang tiap hari bukan masalah. Trade-off yang diambil:
// token lebih sederhana (satu jenis token, bukan access+refresh) dengan
// konsekuensi window kompromi lebih panjang kalau token bocor - dianggap
// cukup untuk sekarang, bisa direvisi ke access+refresh kalau kebutuhannya
// berubah (mis. sesi yang jauh lebih pendek dibutuhkan).
const accessTokenTTL = 24 * time.Hour

// Claims adalah payload yang disisipkan ke JWT - dibaca balik oleh
// middleware.go/RequireAuth setiap request. UserID & Role dipakai handler
// hilir (mis. RequireRole) tanpa perlu query database lagi per-request -
// itulah keuntungan JWT dibanding session-token yang butuh lookup DB/Redis
// tiap kali.
type Claims struct {
	UserID uuid.UUID `json:"user_id"`
	Role   user.Role `json:"role"`
	jwt.RegisteredClaims
}
