// Package taxreport menyediakan GET /reports/tax-summary - agregat
// invoice+payment yang dibutuhkan pemilik bisnis untuk lapor pajak bulanan
// (SPT Masa PPN + rekonsiliasi bukti potong PPh 23). Modul satelit murni
// baca, sama sifatnya dengan internal/dashboard - lihat repository.go
// untuk alasan kenapa polanya (query sqlc baru, bukan reuse repository
// invoice/job) sama dengan keputusan yang sudah diambil di sana.
package taxreport

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/nathan/cnc-pm-backend/internal/dateonly"
)

// ErrInvalidPeriod: period_from tidak boleh setelah period_to - sama
// aturan dan alasannya dengan dashboard.ErrInvalidPeriod (lihat
// service.go modul ini untuk kenapa logic resolusinya DIDUPLIKASI, bukan
// di-reuse dari dashboard, walau pesannya sama).
var ErrInvalidPeriod = errors.New("period_from must not be after period_to")

// Summary adalah bentuk lengkap response GET /reports/tax-summary.
type Summary struct {
	Period Period       `json:"period"`
	PPN    PPNSummary   `json:"ppn"`
	PPh23  PPh23Summary `json:"pph23"`
}

// Period, sama seperti dashboard.Period, SELALU ada (tidak pernah null) -
// value type dateonly.Date, BUKAN pointer, karena field ini tidak pernah
// diserialize sebagai RFC3339 (lihat PR date-only-serialization
// sebelumnya) - endpoint baru ini WAJIB langsung pakai tipe yang benar
// sejak awal, bukan reintroduce bug yang sama.
type Period struct {
	From dateonly.Date `json:"from"`
	To   dateonly.Date `json:"to"`
}

// PPNInvoiceRef adalah satu baris di ppn.invoices - invoice yang jadi
// objek PPN keluaran di period ini (status sent/paid/overdue, SAMA logic
// exclude draft/cancelled dengan dashboard.invoiced_total).
type PPNInvoiceRef struct {
	ID               uuid.UUID       `json:"id"`
	InvoiceNumber    string          `json:"invoice_number"`
	JobCode          string          `json:"job_code"`
	CustomerName     string          `json:"customer_name"`
	Subtotal         decimal.Decimal `json:"subtotal"`
	TaxAmount        decimal.Decimal `json:"tax_amount"`
	NomorFakturPajak *string         `json:"nomor_faktur_pajak"`
}

type PPNSummary struct {
	// TotalPPNKeluaran = SUM(tax_amount) dari Invoices di bawah ini -
	// dihitung Go-side dari baris yang sama (bukan query SUM terpisah),
	// jadi konsisten dengan Invoices by construction, bukan by luck.
	TotalPPNKeluaran decimal.Decimal `json:"total_ppn_keluaran"`
	Invoices         []PPNInvoiceRef `json:"invoices"`
}

// PPh23PaymentRef adalah satu baris di pph23.payments - SEMUA payment
// dalam period, TANPA filter customer_type (keputusan sengaja belum
// final - lihat docs/api-contract.md). CustomerType di-expose APA ADANYA
// dari kolom customers.customer_type ("badan_usaha"/"perorangan") sebagai
// string mentah, BUKAN customer.CustomerType - modul ini sengaja tidak
// import package customer/invoice/job sama sekali (lihat repository.go),
// sama alasannya dengan dashboard.
type PPh23PaymentRef struct {
	ID            uuid.UUID `json:"id"`
	InvoiceID     uuid.UUID `json:"invoice_id"`
	InvoiceNumber string    `json:"invoice_number"`
	CustomerName  string    `json:"customer_name"`
	CustomerType  string    `json:"customer_type"`
	PaymentDate   time.Time `json:"payment_date"`
	// PPh23ShareEstimasi: alokasi PROPORSIONAL dari
	// invoices.pph23_estimated_amount ke payment ini, berdasarkan porsi
	// payment.amount terhadap TOTAL semua payment invoice itu (all-time,
	// bukan cuma yang dalam period) - lihat repository.go untuk rumus
	// dan alasan kenapa proporsional dipilih di atas alternatif yang
	// lebih sederhana.
	PPh23ShareEstimasi  decimal.Decimal `json:"pph23_share_estimasi"`
	BuktiPotongPPh23Ref *string         `json:"bukti_potong_pph23_ref"`
}

type PPh23Summary struct {
	// TotalEstimasi = SUM(pph23_share_estimasi) dari Payments di bawah -
	// sama prinsipnya dengan PPNSummary.TotalPPNKeluaran, dihitung
	// Go-side dari baris yang sama supaya konsisten by construction.
	TotalEstimasi decimal.Decimal   `json:"total_estimasi"`
	Payments      []PPh23PaymentRef `json:"payments"`
}
