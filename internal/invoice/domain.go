package invoice

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Status string

const (
	StatusDraft Status = "draft"
	StatusSent  Status = "sent"
	StatusPaid  Status = "paid"
	// StatusOverdue biasanya diset OTOMATIS oleh Service.MarkOverdue
	// (dipanggil dari ticker reminder yang sama dengan job.ReminderService,
	// lihat cmd/api/main.go/runReminderScheduler), tapi tetap termasuk
	// nilai valid untuk PATCH /invoices/{id}/status manual juga (sesuai
	// api-contract.md) - mis. admin ingin memperbaiki status yang belum
	// sempat ke-set otomatis oleh ticker.
	StatusOverdue   Status = "overdue"
	StatusCancelled Status = "cancelled"
)

func (s Status) Valid() bool {
	switch s {
	case StatusDraft, StatusSent, StatusPaid, StatusOverdue, StatusCancelled:
		return true
	default:
		return false
	}
}

type PaymentMethod string

const (
	PaymentMethodTransfer PaymentMethod = "transfer"
	PaymentMethodCash     PaymentMethod = "cash"
	PaymentMethodOther    PaymentMethod = "other"
)

func (m PaymentMethod) Valid() bool {
	switch m {
	case PaymentMethodTransfer, PaymentMethodCash, PaymentMethodOther:
		return true
	default:
		return false
	}
}

var (
	ErrNotFound = errors.New("invoice not found")
	// ErrJobNotCompleted: kontrak mensyaratkan invoice cuma boleh dibuat dari
	// job yang statusnya sudah "completed" - job yang masih berjalan belum
	// tentu final biayanya.
	ErrJobNotCompleted = errors.New("job must be completed before an invoice can be generated")
	// ErrAlreadyInvoiced: satu job cuma boleh punya satu invoice. Dijaga di
	// dua lapis - di sini (pengecekan eksplisit di dalam transaksi terkunci)
	// dan di database (UNIQUE constraint invoices.job_id), lihat
	// repository.go untuk kenapa dua-duanya dibutuhkan.
	ErrAlreadyInvoiced      = errors.New("job already has an invoice")
	ErrInvalidTaxPercentage = errors.New("tax_percentage must not be negative")
	ErrInvalidStatus        = errors.New("status must be one of: draft, sent, paid, cancelled")
	ErrInvalidFakturPajak   = errors.New("nomor_faktur_pajak is required")
	ErrInvalidAmount        = errors.New("amount must be greater than zero")
	ErrInvalidPaymentMethod = errors.New("payment_method must be one of: transfer, cash, other")
	ErrInvoiceNotPayable    = errors.New("payments can only be recorded against a draft or sent invoice")
)

// Invoice adalah entity domain untuk satu invoice. Semua field angka
// (Subtotal..ExpectedReceivable) dihitung sekali oleh ComputeAmounts saat
// invoice dibuat (lihat repository.go/CreateFromJob) - tidak seperti
// job_costs.subtotal yang generated column, di sini tidak ada input client
// yang perlu "dijaga" (client tidak pernah mengirim subtotal/tax_amount/dst),
// jadi cukup dijamin oleh satu fungsi murni yang diuji unit test
// (ComputeAmounts), bukan constraint database.
type Invoice struct {
	ID                   uuid.UUID       `json:"id"`
	InvoiceNumber        string          `json:"invoice_number"`
	NomorFakturPajak     *string         `json:"nomor_faktur_pajak"`
	JobID                uuid.UUID       `json:"job_id"`
	Subtotal             decimal.Decimal `json:"subtotal"`
	TaxPercentage        decimal.Decimal `json:"tax_percentage"`
	TaxAmount            decimal.Decimal `json:"tax_amount"`
	Total                decimal.Decimal `json:"total"`
	DPPPPh23             decimal.Decimal `json:"dpp_pph23"`
	PPh23Rate            decimal.Decimal `json:"pph23_rate"`
	PPh23EstimatedAmount decimal.Decimal `json:"pph23_estimated_amount"`
	ExpectedReceivable   decimal.Decimal `json:"expected_receivable"`
	Status               Status          `json:"status"`
	DueDate              *time.Time      `json:"due_date"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

// InvoiceListItem adalah bentuk satu baris di GET /invoices - Invoice biasa
// PLUS dua field flat (JobCode, CustomerName) supaya frontend list bisa
// menampilkan identitas manusiawi tanpa request tambahan per baris. Sengaja
// field flat, BUKAN nested object job/customer penuh seperti di GetByID -
// list endpoint butuh tetap ringan (lihat db/queries/invoices.sql ListInvoices).
type InvoiceListItem struct {
	Invoice
	JobCode      string `json:"job_code"`
	CustomerName string `json:"customer_name"`
}

// Payment adalah satu baris pembayaran terhadap sebuah Invoice.
type Payment struct {
	ID                  uuid.UUID       `json:"id"`
	InvoiceID           uuid.UUID       `json:"invoice_id"`
	Amount              decimal.Decimal `json:"amount"`
	PaymentMethod       PaymentMethod   `json:"payment_method"`
	BuktiPotongPPh23Ref *string         `json:"bukti_potong_pph23_ref"`
	Notes               *string         `json:"notes"`
	CreatedAt           time.Time       `json:"created_at"`
}

func (p Payment) Validate() error {
	if p.Amount.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidAmount
	}
	if !p.PaymentMethod.Valid() {
		return ErrInvalidPaymentMethod
	}
	return nil
}

// Amounts menampung hasil ComputeAmounts - dipisah dari Invoice supaya
// fungsinya bisa murni (input -> output, tanpa ID/InvoiceNumber/status yang
// baru ada setelah insert).
type Amounts struct {
	Subtotal             decimal.Decimal
	TaxAmount            decimal.Decimal
	Total                decimal.Decimal
	DPPPPh23             decimal.Decimal
	PPh23EstimatedAmount decimal.Decimal
	ExpectedReceivable   decimal.Decimal
}

// ComputeAmounts adalah jantung perhitungan invoice - sengaja fungsi murni
// (tidak menyentuh database/waktu sekarang) supaya bisa diuji lengkap lewat
// unit test tanpa Postgres, persis aturan di api-contract.md bagian 3:
//
//  1. tax_amount = subtotalAll * taxPercentage / 100 (PPN, selalu dihitung -
//     PT sudah PKP, tidak bisa di-skip)
//  2. total = subtotalAll + tax_amount (nilai resmi di invoice/faktur pajak)
//  3. pph23_estimated_amount = dppPph23 * pph23Rate / 100, HANYA kalau
//     customerIsBadanUsaha - customer perorangan tidak wajib dipotong PPh 23
//  4. expected_receivable = total - pph23_estimated_amount (estimasi uang
//     riil yang diterima, bukan nilai resmi invoice - lihat Catatan Desain #3)
func ComputeAmounts(subtotalAll, dppPph23, taxPercentage, pph23Rate decimal.Decimal, customerIsBadanUsaha bool) Amounts {
	hundred := decimal.NewFromInt(100)

	taxAmount := subtotalAll.Mul(taxPercentage).DivRound(hundred, 2)
	total := subtotalAll.Add(taxAmount)

	pph23Estimated := decimal.Zero
	if customerIsBadanUsaha {
		pph23Estimated = dppPph23.Mul(pph23Rate).DivRound(hundred, 2)
	}
	expectedReceivable := total.Sub(pph23Estimated)

	return Amounts{
		Subtotal:             subtotalAll,
		TaxAmount:            taxAmount,
		Total:                total,
		DPPPPh23:             dppPph23,
		PPh23EstimatedAmount: pph23Estimated,
		ExpectedReceivable:   expectedReceivable,
	}
}
