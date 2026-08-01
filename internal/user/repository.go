package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nathan/cnc-pm-backend/internal/pgconv"
	"github.com/nathan/cnc-pm-backend/internal/repository/sqlcgen"
)

const uniqueViolation = "23505"

// Repository dipakai bukan cuma oleh Service di package ini, tapi juga
// oleh internal/auth (Login perlu GetByEmail) - ini dependency satu arah
// yang disengaja: auth boleh bergantung ke user, user tidak pernah
// bergantung balik ke auth (supaya tidak import cycle).
type Repository interface {
	Create(ctx context.Context, u User) (User, error)
	GetByEmail(ctx context.Context, email string) (User, error)
	GetByID(ctx context.Context, id uuid.UUID) (User, error)
	List(ctx context.Context, role *Role) ([]User, error)
}

type sqlcRepository struct {
	q *sqlcgen.Queries
}

func NewRepository(q *sqlcgen.Queries) Repository {
	return &sqlcRepository{q: q}
}

func (r *sqlcRepository) Create(ctx context.Context, u User) (User, error) {
	row, err := r.q.CreateUser(ctx, sqlcgen.CreateUserParams{
		Name:         u.Name,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		Role:         string(u.Role),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return User{}, ErrDuplicateEmail
		}
		return User{}, fmt.Errorf("insert user: %w", err)
	}
	return fromRow(row), nil
}

func (r *sqlcRepository) GetByEmail(ctx context.Context, email string) (User, error) {
	row, err := r.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, fmt.Errorf("get user by email: %w", err)
	}
	return fromRow(row), nil
}

func (r *sqlcRepository) GetByID(ctx context.Context, id uuid.UUID) (User, error) {
	row, err := r.q.GetUserByID(ctx, pgconv.ToUUID(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, fmt.Errorf("get user by id: %w", err)
	}
	return fromRow(row), nil
}

// List tanpa limit/offset (lihat komentar di ListUsers query) - dipanggil
// dari GET /users, dibatasi role owner/admin di level routing (sama seperti
// Create).
func (r *sqlcRepository) List(ctx context.Context, role *Role) ([]User, error) {
	rows, err := r.q.ListUsers(ctx, toPgTextFromRole(role))
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	users := make([]User, len(rows))
	for i, row := range rows {
		users[i] = fromRow(row)
	}
	return users, nil
}

// toPgTextFromRole tetap khusus di sini (bukan masuk pgconv) karena terikat
// ke tipe Role milik modul ini - pola yang sama dengan
// toPgTextFromCustomerType di internal/customer/repository.go.
func toPgTextFromRole(r *Role) pgtype.Text {
	if r == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: string(*r), Valid: true}
}

func fromRow(row sqlcgen.User) User {
	return User{
		ID:           pgconv.FromUUID(row.ID),
		Name:         row.Name,
		Email:        row.Email,
		PasswordHash: row.PasswordHash,
		Role:         Role(row.Role),
		CreatedAt:    pgconv.FromTimestamptz(row.CreatedAt),
		UpdatedAt:    pgconv.FromTimestamptz(row.UpdatedAt),
		DeletedAt:    pgconv.FromNullableTimestamptz(row.DeletedAt),
	}
}
