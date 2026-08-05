package taxreport

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/nathan/cnc-pm-backend/internal/dateonly"
	"github.com/nathan/cnc-pm-backend/internal/pgconv"
	"github.com/nathan/cnc-pm-backend/internal/repository/sqlcgen"
)

// PeriodInput adalah rentang tanggal yang SUDAH divalidasi dan
// di-default-kan Service (lihat resolvePeriod) - sama pola dengan
// dashboard.PeriodInput.
type PeriodInput struct {
	From        time.Time // tanggal awal, inklusif - jadi Period.From di response
	To          time.Time // tanggal akhir, inklusif - jadi Period.To di response
	ToExclusive time.Time // To + 1 hari - batas atas query DB (created_at < ToExclusive)
}

type Repository interface {
	GetSummary(ctx context.Context, period PeriodInput) (Summary, error)
}

type sqlcRepository struct {
	q *sqlcgen.Queries
}

// NewRepository CUMA butuh *sqlcgen.Queries, BUKAN *pgxpool.Pool (beda
// dari dashboard.NewRepository) - lihat penjelasan lengkap di
// GetSummary kenapa endpoint ini tidak butuh transaksi eksplisit sama
// sekali, apalagi REPEATABLE READ.
func NewRepository(q *sqlcgen.Queries) Repository {
	return &sqlcRepository{q: q}
}

// GetSummary TIDAK dibungkus transaksi eksplisit - dua query di bawah
// (invoices, payments) jalan lewat pool langsung (Read Committed default),
// beda dari dashboard.Repository.GetSummary yang WAJIB REPEATABLE READ.
//
// KENAPA BEDA dari Dashboard, walau sama-sama "endpoint dengan beberapa
// query agregat": REPEATABLE READ di Dashboard dibutuhkan karena
// angka-angkanya SALING HARUS COCOK secara visual ke manusia yang
// melihatnya (invoiced_total harus persis sama dengan jumlah by_status,
// dan seterusnya) - itu jaminan CROSS-QUERY yang Read Committed tidak
// bisa berikan (tiap query dapat snapshot sendiri-sendiri kalau ada
// commit lain di antaranya).
//
// Di sini TIDAK ADA jaminan cross-query seperti itu yang dibutuhkan:
//   - `total_ppn_keluaran` dihitung Go-side dari HASIL query invoices YANG
//     SAMA (bukan query terpisah) - konsisten by construction, satu
//     statement SQL selalu punya snapshot MVCC konsisten sendiri di
//     Postgres (berlaku di isolation level manapun, bukan cuma REPEATABLE
//     READ - ini jaminan dasar MVCC, bukan sesuatu yang perlu level lebih
//     tinggi).
//   - `total_estimasi` sama, dihitung dari hasil query payments yang sama.
//   - ppn (invoice yang diterbitkan) dan pph23 (payment yang diterima)
//     adalah DUA KATEGORI PAJAK YANG BERBEDA secara substansi (PPN soal
//     penerbitan invoice, PPh23 soal penerimaan uang) - keduanya TIDAK
//     pernah diharapkan saling cross-foot/rekonsiliasi satu sama lain
//     secara angka. Tidak masalah kalau query invoices dan query payments
//     "melihat" commit yang sedikit berbeda waktu - beda dari Dashboard
//     yang semua angkanya memang dimaksudkan merepresentasikan SATU
//     momen yang sama.
//
// Konsekuensi praktis: NewRepository tidak perlu *pgxpool.Pool sama
// sekali (tidak ada pgx.BeginFunc), cukup *sqlcgen.Queries yang sudah
// terikat ke pool - persis pola job.Repository.List/invoice.Repository.List
// yang juga tidak butuh transaksi.
func (r *sqlcRepository) GetSummary(ctx context.Context, period PeriodInput) (Summary, error) {
	periodFrom := pgconv.ToTimestamptz(period.From)
	periodToExclusive := pgconv.ToTimestamptz(period.ToExclusive)

	invoiceRows, err := r.q.TaxSummaryInvoices(ctx, sqlcgen.TaxSummaryInvoicesParams{
		PeriodFrom: periodFrom, PeriodToExclusive: periodToExclusive,
	})
	if err != nil {
		return Summary{}, fmt.Errorf("tax summary invoices: %w", err)
	}

	paymentRows, err := r.q.TaxSummaryPayments(ctx, sqlcgen.TaxSummaryPaymentsParams{
		PeriodFrom: periodFrom, PeriodToExclusive: periodToExclusive,
	})
	if err != nil {
		return Summary{}, fmt.Errorf("tax summary payments: %w", err)
	}

	ppnInvoices, totalPPNKeluaran := buildPPNInvoices(invoiceRows)
	pph23Payments, totalEstimasi := buildPPh23Payments(paymentRows)

	return Summary{
		Period: Period{From: dateonly.FromTime(period.From), To: dateonly.FromTime(period.To)},
		PPN: PPNSummary{
			TotalPPNKeluaran: totalPPNKeluaran,
			Invoices:         ppnInvoices,
		},
		PPh23: PPh23Summary{
			TotalEstimasi: totalEstimasi,
			Payments:      pph23Payments,
		},
	}, nil
}

func buildPPNInvoices(rows []sqlcgen.TaxSummaryInvoicesRow) ([]PPNInvoiceRef, decimal.Decimal) {
	refs := make([]PPNInvoiceRef, len(rows))
	total := decimal.Zero
	for i, row := range rows {
		taxAmount := pgconv.FromNumeric(row.TaxAmount)
		total = total.Add(taxAmount)
		refs[i] = PPNInvoiceRef{
			ID:               pgconv.FromUUID(row.ID),
			InvoiceNumber:    row.InvoiceNumber,
			JobCode:          row.JobCode,
			CustomerName:     row.CustomerName,
			Subtotal:         pgconv.FromNumeric(row.Subtotal),
			TaxAmount:        taxAmount,
			NomorFakturPajak: pgconv.FromText(row.NomorFakturPajak),
		}
	}
	return refs, total
}

// buildPPh23Payments mengalokasikan pph23_estimated_amount tiap invoice
// PROPORSIONAL ke tiap payment invoice itu, berdasarkan porsi
// payment.amount terhadap invoice_total_paid (SUM SEMUA payment invoice
// itu, all-time - lihat db/queries/taxreport.sql).
//
// Rumus: share = pph23_estimated_amount * (payment.amount / invoice_total_paid)
//
// Kenapa proporsional, bukan alternatif yang lebih sederhana yang
// dipertimbangkan:
//   - "Taruh semua di payment PERTAMA invoice itu, sisanya 0": salah
//     merepresentasikan KAPAN kewajiban potong itu sebenarnya timbul -
//     bukti potong PPh 23 di dunia nyata biasanya diterbitkan customer
//     PER PEMBAYARAN yang mereka lakukan, proporsional ke jumlah yang
//     mereka bayar saat itu, bukan sekaligus di pembayaran pertama.
//   - "Tampilkan pph23_estimated_amount PENUH di SETIAP payment invoice
//     itu": salah lebih parah - kalau frontend/user menjumlahkan kolom
//     ini across payments, akan double/triple-count nilai yang sama.
//   - Proporsional: SUM(share) across semua payment satu invoice akan
//     PERSIS SAMA dengan pph23_estimated_amount invoice itu (dijamin
//     matematis, modulo rounding kecil di DivRound) - properti yang
//     paling masuk akal untuk direkonsiliasi manual oleh akuntan.
//
// invoice_total_paid dijamin > 0 di titik ini (baris ini datang dari
// INNER JOIN ke payments yang sedang di-scan, jadi invoice ini PASTI
// punya minimal 1 payment) - guard IsZero tetap ada sebagai jaring
// pengaman defensif, bukan karena skenario nyata yang diharapkan terjadi.
func buildPPh23Payments(rows []sqlcgen.TaxSummaryPaymentsRow) ([]PPh23PaymentRef, decimal.Decimal) {
	refs := make([]PPh23PaymentRef, len(rows))
	total := decimal.Zero
	for i, row := range rows {
		paymentAmount := pgconv.FromNumeric(row.PaymentAmount)
		invoiceTotalPaid := pgconv.FromNumeric(row.InvoiceTotalPaid)
		pph23Estimated := pgconv.FromNumeric(row.Pph23EstimatedAmount)

		share := decimal.Zero
		if !invoiceTotalPaid.IsZero() {
			share = pph23Estimated.Mul(paymentAmount).DivRound(invoiceTotalPaid, 2)
		}
		total = total.Add(share)

		refs[i] = PPh23PaymentRef{
			ID:                  pgconv.FromUUID(row.ID),
			InvoiceID:           pgconv.FromUUID(row.InvoiceID),
			InvoiceNumber:       row.InvoiceNumber,
			CustomerName:        row.CustomerName,
			CustomerType:        row.CustomerType,
			PaymentDate:         pgconv.FromTimestamptz(row.PaymentDate),
			PPh23ShareEstimasi:  share,
			BuktiPotongPPh23Ref: pgconv.FromText(row.BuktiPotongPph23Ref),
		}
	}
	return refs, total
}
