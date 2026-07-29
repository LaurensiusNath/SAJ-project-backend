package customer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nathan/cnc-pm-backend/internal/repository/sqlcgen"
)

// Repository adalah kontrak yang dibutuhkan service.go dari layer persistence.
// service.go akan bergantung ke interface ini, BUKAN ke sqlcgen atau pgx
// secara langsung - itu sebabnya semua tipe di signature-nya adalah tipe
// domain (uuid.UUID, Customer, *string), bukan tipe milik driver database.
// ListFilter dipakai bersama oleh List dan Count - keduanya HARUS difilter
// dengan kriteria yang sama persis, supaya meta.total di response benar-benar
// menghitung "total data yang match filter", bukan total keseluruhan tabel.
type ListFilter struct {
	Search       *string
	CustomerType *CustomerType
}

type Repository interface {
	Create(ctx context.Context, c Customer) (Customer, error)
	GetByID(ctx context.Context, id uuid.UUID) (Customer, error)
	List(ctx context.Context, filter ListFilter, limit, offset int32) ([]Customer, error)
	Count(ctx context.Context, filter ListFilter) (int64, error)
	Update(ctx context.Context, c Customer) (Customer, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// sqlcRepository adalah satu-satunya implementasi Repository saat ini,
// membungkus *sqlcgen.Queries yang dihasilkan otomatis dari db/queries/customers.sql.
// Tidak diekspor (huruf kecil) karena caller di luar package ini seharusnya
// cuma bergantung pada interface Repository, dipanggil lewat NewRepository.
type sqlcRepository struct {
	q *sqlcgen.Queries
}

func NewRepository(q *sqlcgen.Queries) Repository {
	return &sqlcRepository{q: q}
}

func (r *sqlcRepository) Create(ctx context.Context, c Customer) (Customer, error) {
	row, err := r.q.CreateCustomer(ctx, sqlcgen.CreateCustomerParams{
		Name:         c.Name,
		CustomerType: string(c.CustomerType),
		Phone:        toPgText(c.Phone),
		Email:        toPgText(c.Email),
		Address:      toPgText(c.Address),
		CompanyName:  toPgText(c.CompanyName),
	})
	if err != nil {
		return Customer{}, fmt.Errorf("insert customer: %w", err)
	}
	return fromRow(row), nil
}

func (r *sqlcRepository) GetByID(ctx context.Context, id uuid.UUID) (Customer, error) {
	row, err := r.q.GetCustomerByID(ctx, toPgUUID(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Customer{}, ErrNotFound
		}
		return Customer{}, fmt.Errorf("get customer by id: %w", err)
	}
	return fromRow(row), nil
}

func (r *sqlcRepository) List(ctx context.Context, filter ListFilter, limit, offset int32) ([]Customer, error) {
	rows, err := r.q.ListCustomers(ctx, sqlcgen.ListCustomersParams{
		Limit:        limit,
		Offset:       offset,
		Search:       toPgText(filter.Search),
		CustomerType: toPgTextFromCustomerType(filter.CustomerType),
	})
	if err != nil {
		return nil, fmt.Errorf("list customers: %w", err)
	}
	customers := make([]Customer, len(rows))
	for i, row := range rows {
		customers[i] = fromRow(row)
	}
	return customers, nil
}

func (r *sqlcRepository) Count(ctx context.Context, filter ListFilter) (int64, error) {
	total, err := r.q.CountCustomers(ctx, sqlcgen.CountCustomersParams{
		Search:       toPgText(filter.Search),
		CustomerType: toPgTextFromCustomerType(filter.CustomerType),
	})
	if err != nil {
		return 0, fmt.Errorf("count customers: %w", err)
	}
	return total, nil
}

func (r *sqlcRepository) Update(ctx context.Context, c Customer) (Customer, error) {
	row, err := r.q.UpdateCustomer(ctx, sqlcgen.UpdateCustomerParams{
		ID:          toPgUUID(c.ID),
		Name:        c.Name,
		Phone:       toPgText(c.Phone),
		Email:       toPgText(c.Email),
		Address:     toPgText(c.Address),
		CompanyName: toPgText(c.CompanyName),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Customer{}, ErrNotFound
		}
		return Customer{}, fmt.Errorf("update customer: %w", err)
	}
	return fromRow(row), nil
}

func (r *sqlcRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.q.SoftDeleteCustomer(ctx, toPgUUID(id)); err != nil {
		return fmt.Errorf("soft delete customer: %w", err)
	}
	return nil
}

// --- konversi tipe domain <-> tipe pgx/sqlcgen ---
//
// sqlc men-generate kolom nullable sebagai pgtype.Text/pgtype.Timestamptz
// (struct dengan field Valid bool), sementara domain.go memakai pointer
// biasa (*string/*time.Time) supaya layer domain tidak perlu import pgx.
// Function-function kecil ini adalah satu-satunya tempat konversi itu terjadi.

func toPgText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func fromPgText(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

func toPgTextFromCustomerType(t *CustomerType) pgtype.Text {
	if t == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: string(*t), Valid: true}
}

func toPgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func fromPgUUID(id pgtype.UUID) uuid.UUID {
	return uuid.UUID(id.Bytes)
}

func fromPgTimestamptzPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func fromRow(row sqlcgen.Customer) Customer {
	return Customer{
		ID:           fromPgUUID(row.ID),
		Name:         row.Name,
		CustomerType: CustomerType(row.CustomerType),
		Phone:        fromPgText(row.Phone),
		Email:        fromPgText(row.Email),
		Address:      fromPgText(row.Address),
		CompanyName:  fromPgText(row.CompanyName),
		CreatedAt:    row.CreatedAt.Time,
		UpdatedAt:    row.UpdatedAt.Time,
		DeletedAt:    fromPgTimestamptzPtr(row.DeletedAt),
	}
}
