package invoice

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// CreateInput.TaxPercentage pakai pointer - kalau nil, repository.go
// memakai company_settings.default_tax_percentage (lihat api-contract.md:
// "bisa di-override per invoice kalau perlu kasus khusus"). PPh 23 rate
// tidak punya jalur override sama sekali di kontrak, jadi selalu dari
// company_settings.
type CreateInput struct {
	TaxPercentage *decimal.Decimal
	DueDate       *time.Time
}

func (s *Service) CreateFromJob(ctx context.Context, jobID uuid.UUID, in CreateInput) (Invoice, error) {
	if in.TaxPercentage != nil && in.TaxPercentage.IsNegative() {
		return Invoice{}, ErrInvalidTaxPercentage
	}

	created, err := s.repo.CreateFromJob(ctx, CreateFromJobInput{
		JobID:         jobID,
		TaxPercentage: in.TaxPercentage,
		DueDate:       in.DueDate,
	})
	if err != nil {
		return Invoice{}, err
	}
	return created, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (Invoice, error) {
	return s.repo.GetByID(ctx, id)
}

type ListParams struct {
	Status *Status
	Page   int32
	Limit  int32
}

type ListResult struct {
	Invoices []Invoice
	Total    int64
}

func (s *Service) List(ctx context.Context, p ListParams) (ListResult, error) {
	if p.Status != nil && !p.Status.Valid() {
		return ListResult{}, ErrInvalidStatus
	}

	limit := p.Limit
	if limit <= 0 {
		limit = 20
	}
	page := p.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	filter := ListFilter{Status: p.Status}
	invoices, err := s.repo.List(ctx, filter, limit, offset)
	if err != nil {
		return ListResult{}, fmt.Errorf("list invoices: %w", err)
	}
	total, err := s.repo.Count(ctx, filter)
	if err != nil {
		return ListResult{}, fmt.Errorf("count invoices: %w", err)
	}
	return ListResult{Invoices: invoices, Total: total}, nil
}

func (s *Service) UpdateFakturPajak(ctx context.Context, id uuid.UUID, nomorFakturPajak string) (Invoice, error) {
	if nomorFakturPajak == "" {
		return Invoice{}, ErrInvalidFakturPajak
	}
	updated, err := s.repo.UpdateFakturPajak(ctx, id, nomorFakturPajak)
	if err != nil {
		return Invoice{}, fmt.Errorf("update faktur pajak: %w", err)
	}
	return updated, nil
}

func (s *Service) UpdateStatus(ctx context.Context, id uuid.UUID, status Status) (Invoice, error) {
	if !status.Valid() {
		return Invoice{}, ErrInvalidStatus
	}
	updated, err := s.repo.UpdateStatus(ctx, id, status)
	if err != nil {
		return Invoice{}, fmt.Errorf("update invoice status: %w", err)
	}
	return updated, nil
}

type CreatePaymentInput struct {
	Amount              decimal.Decimal
	PaymentMethod       PaymentMethod
	BuktiPotongPPh23Ref *string
	Notes               *string
}

func (s *Service) RecordPayment(ctx context.Context, invoiceID uuid.UUID, in CreatePaymentInput) (Payment, error) {
	p := Payment{
		InvoiceID:           invoiceID,
		Amount:              in.Amount,
		PaymentMethod:       in.PaymentMethod,
		BuktiPotongPPh23Ref: in.BuktiPotongPPh23Ref,
		Notes:               in.Notes,
	}
	if err := p.Validate(); err != nil {
		return Payment{}, err
	}

	created, err := s.repo.RecordPayment(ctx, RecordPaymentInput{
		InvoiceID:           invoiceID,
		Amount:              in.Amount,
		PaymentMethod:       in.PaymentMethod,
		BuktiPotongPPh23Ref: in.BuktiPotongPPh23Ref,
		Notes:               in.Notes,
	})
	if err != nil {
		return Payment{}, fmt.Errorf("record payment: %w", err)
	}
	return created, nil
}

func (s *Service) ListPayments(ctx context.Context, invoiceID uuid.UUID) ([]Payment, error) {
	payments, err := s.repo.ListPayments(ctx, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("list payments: %w", err)
	}
	return payments, nil
}

// MarkOverdue dipanggil dari ticker reminder yang sama dengan
// job.ReminderService (lihat cmd/api/main.go/runReminderScheduler) - BUKAN
// infrastruktur terjadwal baru. invoice package tidak bisa memiliki
// "ReminderService"-nya sendiri yang meniru job.ReminderService secara
// langsung, karena invoice SUDAH mengimpor job (invoice.Repository memakai
// job.NewRepository di dalam transaksi CreateFromJob) - kalau job balik
// mengimpor invoice untuk method ini, akan jadi circular import. main.go
// (yang sudah mengimpor keduanya) yang menjembatani kedua pemanggilan itu
// di ticker yang sama.
func (s *Service) MarkOverdue(ctx context.Context) ([]Invoice, error) {
	overdue, err := s.repo.MarkOverdue(ctx)
	if err != nil {
		return nil, fmt.Errorf("mark overdue invoices: %w", err)
	}
	return overdue, nil
}
