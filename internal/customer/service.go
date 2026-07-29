package customer

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Service berisi business logic modul Customer. Ia bergantung ke interface
// Repository (bukan implementasi konkretnya) - itu yang membuat Service ini
// bisa di-unit-test tanpa database sungguhan, lihat service_test.go.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// CreateInput adalah data yang dibutuhkan untuk membuat customer baru.
// Dipisah dari struct Customer supaya handler tidak perlu (dan tidak boleh)
// mengisi field yang harusnya dihitung backend, seperti ID atau CreatedAt.
type CreateInput struct {
	Name         string
	CustomerType CustomerType
	Phone        *string
	Email        *string
	Address      *string
	CompanyName  *string
}

func (s *Service) Create(ctx context.Context, in CreateInput) (Customer, error) {
	c := Customer{
		Name:         in.Name,
		CustomerType: in.CustomerType,
		Phone:        in.Phone,
		Email:        in.Email,
		Address:      in.Address,
		CompanyName:  in.CompanyName,
	}
	if err := c.Validate(); err != nil {
		return Customer{}, err
	}

	created, err := s.repo.Create(ctx, c)
	if err != nil {
		return Customer{}, fmt.Errorf("create customer: %w", err)
	}
	return created, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (Customer, error) {
	return s.repo.GetByID(ctx, id)
}

// ListParams memakai Page (1-based, seperti di query param API) bukan Offset
// langsung - konversi ke offset ada di sini, supaya handler tidak perlu tahu
// aritmatika pagination.
type ListParams struct {
	Search *string
	Page   int32
	Limit  int32
}

func (s *Service) List(ctx context.Context, p ListParams) ([]Customer, error) {
	limit := p.Limit
	if limit <= 0 {
		limit = 20
	}
	page := p.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	customers, err := s.repo.List(ctx, p.Search, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list customers: %w", err)
	}
	return customers, nil
}

// UpdateInput sengaja tidak menyertakan CustomerType - sesuai UpdateCustomer
// query di db/queries/customers.sql, jenis customer tidak bisa diubah lewat
// endpoint update (kalau salah input saat create, perlu proses lain).
type UpdateInput struct {
	ID          uuid.UUID
	Name        string
	Phone       *string
	Email       *string
	Address     *string
	CompanyName *string
}

func (s *Service) Update(ctx context.Context, in UpdateInput) (Customer, error) {
	existing, err := s.repo.GetByID(ctx, in.ID)
	if err != nil {
		return Customer{}, err
	}

	existing.Name = in.Name
	existing.Phone = in.Phone
	existing.Email = in.Email
	existing.Address = in.Address
	existing.CompanyName = in.CompanyName
	if err := existing.Validate(); err != nil {
		return Customer{}, err
	}

	updated, err := s.repo.Update(ctx, existing)
	if err != nil {
		return Customer{}, fmt.Errorf("update customer: %w", err)
	}
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete customer: %w", err)
	}
	return nil
}
