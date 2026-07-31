package auth

import (
	"context"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/nathan/cnc-pm-backend/internal/user"
)

type fakeUserRepository struct {
	usersByEmail map[string]user.User
}

func newFakeUserRepository() *fakeUserRepository {
	return &fakeUserRepository{usersByEmail: make(map[string]user.User)}
}

func (f *fakeUserRepository) Create(_ context.Context, u user.User) (user.User, error) {
	f.usersByEmail[u.Email] = u
	return u, nil
}

func (f *fakeUserRepository) GetByEmail(_ context.Context, email string) (user.User, error) {
	u, ok := f.usersByEmail[email]
	if !ok {
		return user.User{}, user.ErrNotFound
	}
	return u, nil
}

func (f *fakeUserRepository) GetByID(_ context.Context, id uuid.UUID) (user.User, error) {
	for _, u := range f.usersByEmail {
		if u.ID == id {
			return u, nil
		}
	}
	return user.User{}, user.ErrNotFound
}

func seedUser(t *testing.T, repo *fakeUserRepository, email, password string, role user.Role) user.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	require.NoError(t, err)
	u := user.User{ID: uuid.New(), Name: "Test User", Email: email, PasswordHash: string(hash), Role: role}
	created, err := repo.Create(context.Background(), u)
	require.NoError(t, err)
	return created
}

func TestService_Login_Success(t *testing.T) {
	repo := newFakeUserRepository()
	seeded := seedUser(t, repo, "owner@cncservis.local", "ChangeMe123!", user.RoleOwner)
	svc := NewService(repo, "test-secret")

	tokenString, loggedInUser, err := svc.Login(context.Background(), "owner@cncservis.local", "ChangeMe123!")

	require.NoError(t, err)
	assert.NotEmpty(t, tokenString)
	assert.Equal(t, seeded.ID, loggedInUser.ID)
	assert.Equal(t, seeded.Email, loggedInUser.Email)

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(*jwt.Token) (interface{}, error) {
		return []byte("test-secret"), nil
	})
	require.NoError(t, err)
	assert.True(t, token.Valid)
	assert.Equal(t, seeded.ID, claims.UserID)
	assert.Equal(t, user.RoleOwner, claims.Role)
}

func TestService_Login_WrongPassword(t *testing.T) {
	repo := newFakeUserRepository()
	seedUser(t, repo, "owner@cncservis.local", "ChangeMe123!", user.RoleOwner)
	svc := NewService(repo, "test-secret")

	_, _, err := svc.Login(context.Background(), "owner@cncservis.local", "wrong-password")

	require.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestService_Login_UnknownEmail(t *testing.T) {
	svc := NewService(newFakeUserRepository(), "test-secret")

	_, _, err := svc.Login(context.Background(), "tidak-ada@cncservis.local", "apapun123")

	require.ErrorIs(t, err, ErrInvalidCredentials, "unknown email must return the same error as wrong password, not leak which emails exist")
}
