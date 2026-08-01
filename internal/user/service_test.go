package user

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

type fakeRepository struct {
	usersByEmail map[string]User
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{usersByEmail: make(map[string]User)}
}

func (f *fakeRepository) Create(_ context.Context, u User) (User, error) {
	if _, exists := f.usersByEmail[u.Email]; exists {
		return User{}, ErrDuplicateEmail
	}
	u.ID = uuid.New()
	u.CreatedAt = time.Now()
	u.UpdatedAt = time.Now()
	f.usersByEmail[u.Email] = u
	return u, nil
}

func (f *fakeRepository) GetByEmail(_ context.Context, email string) (User, error) {
	u, ok := f.usersByEmail[email]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}

func (f *fakeRepository) List(_ context.Context, role *Role) ([]User, error) {
	var result []User
	for _, u := range f.usersByEmail {
		if role != nil && u.Role != *role {
			continue
		}
		result = append(result, u)
	}
	return result, nil
}

func (f *fakeRepository) GetByID(_ context.Context, id uuid.UUID) (User, error) {
	for _, u := range f.usersByEmail {
		if u.ID == id {
			return u, nil
		}
	}
	return User{}, ErrNotFound
}

func TestService_Create(t *testing.T) {
	testCases := []struct {
		name    string
		input   CreateInput
		wantErr error
	}{
		{
			name:  "valid user",
			input: CreateInput{Name: "Teknisi Satu", Email: "teknisi1@cncservis.local", Password: "password123", Role: RoleTeknisi},
		},
		{
			name:    "short password is rejected",
			input:   CreateInput{Name: "Teknisi Dua", Email: "teknisi2@cncservis.local", Password: "short", Role: RoleTeknisi},
			wantErr: ErrInvalidPassword,
		},
		{
			name:    "invalid role is rejected",
			input:   CreateInput{Name: "X", Email: "x@cncservis.local", Password: "password123", Role: Role("bukan-valid")},
			wantErr: ErrInvalidRole,
		},
		{
			name:    "empty name is rejected",
			input:   CreateInput{Name: "", Email: "x@cncservis.local", Password: "password123", Role: RoleAdmin},
			wantErr: ErrInvalidName,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(newFakeRepository())

			got, err := svc.Create(context.Background(), tc.input)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.NotEqual(t, uuid.Nil, got.ID)
			assert.NotEmpty(t, got.PasswordHash, "password should be hashed and stored")
			assert.NotEqual(t, tc.input.Password, got.PasswordHash, "password_hash must never equal the raw password")
			require.NoError(t, bcrypt.CompareHashAndPassword([]byte(got.PasswordHash), []byte(tc.input.Password)),
				"stored hash must verify against the original password")
		})
	}
}

func TestService_List(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ctx := context.Background()
	_, err := svc.Create(ctx, CreateInput{Name: "Owner", Email: "owner@cncservis.local", Password: "password123", Role: RoleOwner})
	require.NoError(t, err)
	_, err = svc.Create(ctx, CreateInput{Name: "Teknisi Satu", Email: "t1@cncservis.local", Password: "password123", Role: RoleTeknisi})
	require.NoError(t, err)
	_, err = svc.Create(ctx, CreateInput{Name: "Teknisi Dua", Email: "t2@cncservis.local", Password: "password123", Role: RoleTeknisi})
	require.NoError(t, err)

	t.Run("no filter returns all users", func(t *testing.T) {
		got, err := svc.List(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, got, 3)
	})

	t.Run("filter by role - dipakai frontend untuk dropdown assign teknisi", func(t *testing.T) {
		teknisi := RoleTeknisi
		got, err := svc.List(ctx, &teknisi)
		require.NoError(t, err)
		assert.Len(t, got, 2)
		for _, u := range got {
			assert.Equal(t, RoleTeknisi, u.Role)
		}
	})

	t.Run("invalid role filter is rejected", func(t *testing.T) {
		invalid := Role("bukan-valid")
		_, err := svc.List(ctx, &invalid)
		require.ErrorIs(t, err, ErrInvalidRole)
	})
}

func TestService_Create_DuplicateEmail(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	in := CreateInput{Name: "A", Email: "dup@cncservis.local", Password: "password123", Role: RoleAdmin}

	_, err := svc.Create(context.Background(), in)
	require.NoError(t, err)

	_, err = svc.Create(context.Background(), in)
	require.ErrorIs(t, err, ErrDuplicateEmail)
}
