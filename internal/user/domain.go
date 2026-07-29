package user

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Role merepresentasikan kolom users.role - pola typed-string enum yang
// sama seperti CustomerType/JobStatus/dst di modul lain.
type Role string

const (
	RoleOwner   Role = "owner"
	RoleAdmin   Role = "admin"
	RoleTeknisi Role = "teknisi"
)

func (r Role) Valid() bool {
	switch r {
	case RoleOwner, RoleAdmin, RoleTeknisi:
		return true
	default:
		return false
	}
}

var (
	ErrNotFound        = errors.New("user not found")
	ErrInvalidName     = errors.New("name is required")
	ErrInvalidEmail    = errors.New("email is required")
	ErrInvalidRole     = errors.New("role must be one of: owner, admin, teknisi")
	ErrInvalidPassword = errors.New("password must be at least 8 characters")
	ErrDuplicateEmail  = errors.New("email is already registered")
)

// User adalah entity domain untuk satu akun staf (owner/admin/teknisi).
// PasswordHash sengaja diberi `json:"-"` - tidak boleh pernah ikut
// ter-serialize ke response manapun walau tidak sengaja, beda dari field
// lain yang memang boleh tampil di API.
type User struct {
	ID           uuid.UUID  `json:"id"`
	Name         string     `json:"name"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"`
	Role         Role       `json:"role"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at"`
}

func (u User) Validate() error {
	if u.Name == "" {
		return ErrInvalidName
	}
	if u.Email == "" {
		return ErrInvalidEmail
	}
	if !u.Role.Valid() {
		return ErrInvalidRole
	}
	return nil
}
