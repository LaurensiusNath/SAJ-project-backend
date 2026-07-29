package invoice

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRepository di sini SENGAJA tidak mereplikasi logika transaksi/locking
// yang sesungguhnya (CreateFromJob/RecordPayment yang asli ada di
// repository.go, dijalankan lewat Postgres) - unit test ini cuma menguji
// validasi dan alur Service, bukan mekanisme concurrency-nya. Untuk itu,
// lihat pengujian manual dengan goroutine terhadap Postgres asli yang
// didokumentasikan terpisah (bukan bagian dari `go test ./...`, karena CI
// belum punya database - lihat catatan ADR/README).
type fakeRepository struct {
	invoices map[uuid.UUID]Invoice
	payments map[uuid.UUID][]Payment
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		invoices: make(map[uuid.UUID]Invoice),
		payments: make(map[uuid.UUID][]Payment),
	}
}

func (f *fakeRepository) CreateFromJob(_ context.Context, in CreateFromJobInput) (Invoice, error) {
	for _, existing := range f.invoices {
		if existing.JobID == in.JobID {
			return Invoice{}, ErrAlreadyInvoiced
		}
	}
	taxPercentage := decimal.NewFromInt(11)
	if in.TaxPercentage != nil {
		taxPercentage = *in.TaxPercentage
	}
	inv := Invoice{
		ID:            uuid.New(),
		InvoiceNumber: "INV-2026-0001",
		JobID:         in.JobID,
		TaxPercentage: taxPercentage,
		Status:        StatusDraft,
		DueDate:       in.DueDate,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	f.invoices[inv.ID] = inv
	return inv, nil
}

func (f *fakeRepository) GetByID(_ context.Context, id uuid.UUID) (Invoice, error) {
	inv, ok := f.invoices[id]
	if !ok {
		return Invoice{}, ErrNotFound
	}
	return inv, nil
}

func (f *fakeRepository) List(_ context.Context, filter ListFilter, _, _ int32) ([]Invoice, error) {
	var result []Invoice
	for _, inv := range f.invoices {
		if filter.Status != nil && inv.Status != *filter.Status {
			continue
		}
		result = append(result, inv)
	}
	return result, nil
}

func (f *fakeRepository) Count(_ context.Context, filter ListFilter) (int64, error) {
	invoices, _ := f.List(context.Background(), filter, 0, 0)
	return int64(len(invoices)), nil
}

func (f *fakeRepository) UpdateFakturPajak(_ context.Context, id uuid.UUID, nomorFakturPajak string) (Invoice, error) {
	inv, ok := f.invoices[id]
	if !ok {
		return Invoice{}, ErrNotFound
	}
	inv.NomorFakturPajak = &nomorFakturPajak
	f.invoices[id] = inv
	return inv, nil
}

func (f *fakeRepository) UpdateStatus(_ context.Context, id uuid.UUID, status Status) (Invoice, error) {
	inv, ok := f.invoices[id]
	if !ok {
		return Invoice{}, ErrNotFound
	}
	inv.Status = status
	f.invoices[id] = inv
	return inv, nil
}

func (f *fakeRepository) RecordPayment(_ context.Context, in RecordPaymentInput) (Payment, error) {
	if _, ok := f.invoices[in.InvoiceID]; !ok {
		return Payment{}, ErrNotFound
	}
	p := Payment{
		ID:                  uuid.New(),
		InvoiceID:           in.InvoiceID,
		Amount:              in.Amount,
		PaymentMethod:       in.PaymentMethod,
		BuktiPotongPPh23Ref: in.BuktiPotongPPh23Ref,
		Notes:               in.Notes,
		CreatedAt:           time.Now(),
	}
	f.payments[in.InvoiceID] = append(f.payments[in.InvoiceID], p)
	return p, nil
}

func (f *fakeRepository) ListPayments(_ context.Context, invoiceID uuid.UUID) ([]Payment, error) {
	return f.payments[invoiceID], nil
}

func TestService_CreateFromJob(t *testing.T) {
	negative := decimal.NewFromInt(-1)
	valid := decimal.NewFromInt(11)

	testCases := []struct {
		name    string
		input   CreateInput
		wantErr error
	}{
		{name: "valid tax_percentage override", input: CreateInput{TaxPercentage: &valid}},
		{name: "nil tax_percentage falls back to repository default", input: CreateInput{}},
		{name: "negative tax_percentage is rejected", input: CreateInput{TaxPercentage: &negative}, wantErr: ErrInvalidTaxPercentage},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(newFakeRepository())

			got, err := svc.CreateFromJob(context.Background(), uuid.New(), tc.input)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, StatusDraft, got.Status)
		})
	}
}

func TestService_CreateFromJob_AlreadyInvoiced(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	jobID := uuid.New()

	_, err := svc.CreateFromJob(context.Background(), jobID, CreateInput{})
	require.NoError(t, err)

	_, err = svc.CreateFromJob(context.Background(), jobID, CreateInput{})
	require.ErrorIs(t, err, ErrAlreadyInvoiced)
}

func TestService_List_InvalidStatus(t *testing.T) {
	svc := NewService(newFakeRepository())

	invalid := Status("bukan-valid")
	_, err := svc.List(context.Background(), ListParams{Status: &invalid})

	require.ErrorIs(t, err, ErrInvalidStatus)
}

func TestService_UpdateFakturPajak_EmptyRejected(t *testing.T) {
	svc := NewService(newFakeRepository())

	_, err := svc.UpdateFakturPajak(context.Background(), uuid.New(), "")

	require.ErrorIs(t, err, ErrInvalidFakturPajak)
}

func TestService_UpdateStatus_InvalidRejected(t *testing.T) {
	svc := NewService(newFakeRepository())

	_, err := svc.UpdateStatus(context.Background(), uuid.New(), Status("bukan-valid"))

	require.ErrorIs(t, err, ErrInvalidStatus)
}

func TestService_RecordPayment(t *testing.T) {
	testCases := []struct {
		name    string
		input   CreatePaymentInput
		wantErr error
	}{
		{
			name: "valid transfer payment",
			input: CreatePaymentInput{
				Amount: decimal.NewFromInt(1649000), PaymentMethod: PaymentMethodTransfer,
			},
		},
		{
			name:    "zero amount is rejected",
			input:   CreatePaymentInput{Amount: decimal.Zero, PaymentMethod: PaymentMethodTransfer},
			wantErr: ErrInvalidAmount,
		},
		{
			name:    "invalid payment_method is rejected",
			input:   CreatePaymentInput{Amount: decimal.NewFromInt(1000), PaymentMethod: PaymentMethod("bukan-valid")},
			wantErr: ErrInvalidPaymentMethod,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepository()
			svc := NewService(repo)
			created, err := svc.CreateFromJob(context.Background(), uuid.New(), CreateInput{})
			require.NoError(t, err)

			_, err = svc.RecordPayment(context.Background(), created.ID, tc.input)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
