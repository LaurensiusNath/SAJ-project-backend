package user

import (
	"context"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

type CreateInput struct {
	Name     string
	Email    string
	Password string
	Role     Role
}

// Create dipanggil dari POST /users, dibatasi role owner/admin di level
// routing (lihat cmd/api/main.go) - Service sendiri tidak tahu-menahu soal
// siapa yang memanggil, cuma soal validasi & hashing password.
func (s *Service) Create(ctx context.Context, in CreateInput) (User, error) {
	if len(in.Password) < 8 {
		return User{}, ErrInvalidPassword
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, fmt.Errorf("hash password: %w", err)
	}

	u := User{
		Name:         in.Name,
		Email:        in.Email,
		PasswordHash: string(hash),
		Role:         in.Role,
	}
	if err := u.Validate(); err != nil {
		return User{}, err
	}

	created, err := s.repo.Create(ctx, u)
	if err != nil {
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return created, nil
}

// List dipanggil dari GET /users, dibatasi role owner/admin di level
// routing (sama seperti Create). roleFilter nil berarti tanpa filter (semua
// role) - kalau diisi, harus salah satu role yang valid (mis. dipakai
// frontend untuk dropdown assign teknisi: ?role=teknisi).
func (s *Service) List(ctx context.Context, roleFilter *Role) ([]User, error) {
	if roleFilter != nil && !roleFilter.Valid() {
		return nil, ErrInvalidRole
	}
	users, err := s.repo.List(ctx, roleFilter)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return users, nil
}
