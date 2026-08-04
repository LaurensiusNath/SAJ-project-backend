package dashboard

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ErrInvalidPeriod: period_from tidak boleh setelah period_to. Divalidasi di
// Service (bukan Handler) karena ini aturan bisnis tentang RENTANG yang
// masuk akal, bukan sekadar format tanggal (itu tugas Handler, sama seperti
// parseOptionalDate di modul invoice/job).
var ErrInvalidPeriod = errors.New("period_from must not be after period_to")

// Summary adalah bentuk lengkap response GET /dashboard/summary.
type Summary struct {
	Financial FinancialSummary `json:"financial"`
	Jobs      JobsSummary      `json:"jobs"`
}

// Period selalu tanggal kalender INKLUSIF kedua ujung (From dan To
// sama-sama termasuk) - lihat Service.resolvePeriod untuk bagaimana ini
// diterjemahkan jadi batas query [From, To+1hari) di database.
type Period struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// InvoiceStatusAmount adalah satu baris breakdown financial.by_status.
type InvoiceStatusAmount struct {
	Count int64           `json:"count"`
	Total decimal.Decimal `json:"total"`
}

type FinancialSummary struct {
	Period Period `json:"period"`
	// InvoicedTotal: SUM(invoices.total) WHERE created_at dalam period.
	InvoicedTotal decimal.Decimal `json:"invoiced_total"`
	// ReceivedTotal: SUM(payments.amount) WHERE payments.created_at dalam period.
	ReceivedTotal decimal.Decimal `json:"received_total"`
	// OutstandingTotal: saldo tagihan yang BELUM lunas SAAT INI (status
	// sent/overdue) - sengaja TIDAK dibatasi period, ini posisi sekarang,
	// bukan arus kas periode. Bisa jadi angka negatif kalau ada invoice
	// yang secara manual di-PATCH statusnya balik ke sent/overdue setelah
	// terlanjur lunas (lihat docs/api-contract.md Catatan Desain & Keputusan
	// Teknis Kunci: "PATCH /invoices/{id}/status tanpa state-machine/
	// transition guard") - sengaja TIDAK
	// di-clamp ke 0, angka negatif di sini justru sinyal berguna kalau itu
	// terjadi, bukan sesuatu yang perlu disembunyikan.
	OutstandingTotal decimal.Decimal `json:"outstanding_total"`
	// ByStatus SENGAJA struct, bukan map[string]InvoiceStatusAmount -
	// menjamin ke-5 status invoice selalu muncul di response walau count-nya
	// 0, tanpa logic zero-fill manual yang gampang lupa satu status kalau
	// suatu saat ada status invoice baru ditambahkan.
	ByStatus FinancialByStatus `json:"by_status"`
}

type FinancialByStatus struct {
	Draft     InvoiceStatusAmount `json:"draft"`
	Sent      InvoiceStatusAmount `json:"sent"`
	Paid      InvoiceStatusAmount `json:"paid"`
	Overdue   InvoiceStatusAmount `json:"overdue"`
	Cancelled InvoiceStatusAmount `json:"cancelled"`
}

type JobsSummary struct {
	// ByStatus ALL-TIME (bukan per-period) - snapshot kondisi backlog job
	// sekarang, bukan arus job baru di suatu rentang waktu.
	ByStatus JobsByStatus `json:"by_status"`
	// Upcoming7Days: scheduled_date dari HARI INI s/d +7 HARI (inklusif
	// kedua ujung, jadi 8 hari kalender), status belum selesai/batal.
	Upcoming7Days []ScheduledJobRef `json:"upcoming_7_days"`
	// OverdueScheduled: scheduled_date < hari ini, status BUKAN
	// completed/cancelled - job yang seharusnya sudah dikerjakan tapi belum.
	OverdueScheduled []ScheduledJobRef `json:"overdue_scheduled"`
}

type JobsByStatus struct {
	Requested  int64 `json:"requested"`
	Scheduled  int64 `json:"scheduled"`
	InProgress int64 `json:"in_progress"`
	Completed  int64 `json:"completed"`
	Cancelled  int64 `json:"cancelled"`
}

// ScheduledJobRef adalah satu baris di upcoming_7_days/overdue_scheduled -
// job_code+customer_name flat lewat JOIN jobs+customers, reuse pola yang
// sama dengan InvoiceListItem di modul invoice (lihat db/queries/dashboard.sql).
// ScheduledDate tetap pointer (bukan value) untuk konsisten dengan
// job.Job.ScheduledDate yang nullable di skema, walau di dua list ini
// query-nya sendiri menjamin baris yang dikembalikan selalu punya
// scheduled_date terisi.
type ScheduledJobRef struct {
	ID            uuid.UUID  `json:"id"`
	JobCode       string     `json:"job_code"`
	CustomerName  string     `json:"customer_name"`
	ScheduledDate *time.Time `json:"scheduled_date"`
}
