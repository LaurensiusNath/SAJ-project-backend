package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/nathan/cnc-pm-backend/internal/user"
)

// Service menangani login (menerbitkan token) sekaligus jadi tempat
// RequireAuth/RequireRole hidup (middleware.go, file lain di package ini) -
// keduanya berbagi secret yang sama untuk menandatangani & memvalidasi JWT.
type Service struct {
	userRepo user.Repository
	secret   []byte
}

// NewService menerima secret sebagai string, bukan baca env sendiri -
// package ini tidak tahu-menahu soal config, main.go yang inject dari
// cfg.JWTSecret (lihat internal/config/config.go - sengaja tidak ada
// default yang aman untuk secret ini).
func NewService(userRepo user.Repository, secret string) *Service {
	return &Service{userRepo: userRepo, secret: []byte(secret)}
}

func (s *Service) Login(ctx context.Context, email, password string) (string, error) {
	u, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return "", ErrInvalidCredentials
		}
		return "", fmt.Errorf("get user by email: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return "", ErrInvalidCredentials
	}

	now := time.Now()
	claims := Claims{
		UserID: u.ID,
		Role:   u.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(accessTokenTTL)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}
