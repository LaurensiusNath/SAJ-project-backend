package dashboard

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nathan/cnc-pm-backend/internal/pgconv"
	"github.com/nathan/cnc-pm-backend/internal/repository/sqlcgen"
)

// PeriodInput adalah rentang tanggal yang SUDAH divalidasi dan di-default-kan
// Service (lihat resolvePeriod) - Repository tidak pernah menerjemahkan
// ulang "period_to opsional" dst, cuma memakai apa yang dikirim.
type PeriodInput struct {
	From        time.Time // tanggal awal, inklusif - jadi financial.period.from di response
	To          time.Time // tanggal akhir, inklusif - jadi financial.period.to di response
	ToExclusive time.Time // To + 1 hari - batas atas query DB (created_at < ToExclusive)
}

type Repository interface {
	// GetSummary membungkus SEMUA query (financial + jobs) dalam SATU
	// transaksi REPEATABLE READ read-only - lihat penjelasan lengkap di
	// badan fungsi sqlcRepository.GetSummary soal kenapa ini WAJIB untuk
	// modul Dashboard, beda dari modul CRUD lain yang cukup Read Committed
	// (default Postgres).
	GetSummary(ctx context.Context, period PeriodInput) (Summary, error)
}

type sqlcRepository struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

// NewRepository butuh *pgxpool.Pool (bukan cuma Queries) - GetSummary
// membuka transaksinya sendiri dengan isolation level custom, pola yang
// sama dengan invoice.NewRepository/job.NewRepository.
func NewRepository(pool *pgxpool.Pool, q *sqlcgen.Queries) Repository {
	return &sqlcRepository{pool: pool, q: q}
}

// GetSummary adalah jawaban langsung untuk catatan lama di CLAUDE.md soal
// "isolation level perlu naik ke Repeatable Read/Serializable untuk modul
// Dashboard yang butuh snapshot konsisten dari banyak tabel".
//
// KENAPA Read Committed (default Postgres) TIDAK CUKUP di sini:
//
// Read Committed memberi setiap STATEMENT snapshot-nya sendiri-sendiri, yang
// diambil saat statement itu MULAI dieksekusi - bukan snapshot per
// transaksi. Endpoint ini menjalankan banyak SELECT terpisah (invoiced_total,
// received_total, outstanding_total, by_status invoice, by_status job,
// upcoming/overdue job). Kalau masing-masing pakai Read Committed biasa dan
// ada request LAIN yang commit sesuatu DI TENGAH rangkaian SELECT ini
// (mis. RecordPayment masuk tepat setelah invoiced_total dibaca tapi
// sebelum received_total dibaca) - dua angka yang tampil di satu response
// yang sama bisa "berasal dari dua titik waktu berbeda". Tidak ada satupun
// angka yang SALAH secara individual (masing-masing benar pada saat
// dibaca), tapi digabung jadi satu dashboard, angkanya bisa terlihat tidak
// masuk akal ke user (mis. outstanding_total belum mencerminkan payment
// yang justru sudah tercermin di received_total). Ini beda kelas masalah
// dari lost-update di RecordPayment (yang butuh row lock/pessimistic
// locking) - di sini tidak ada row yang ditulis dua kali, masalahnya murni
// KONSISTENSI ANTAR-QUERY dalam satu view.
//
// REPEATABLE READ (level isolasi Postgres di atas Read Committed) mengambil
// SATU snapshot di query PERTAMA transaksi ini, lalu seluruh query
// berikutnya di transaksi yang sama WAJIB memakai snapshot itu juga -
// commit dari transaksi lain yang terjadi di tengah-tengah kita membaca
// jadi tidak terlihat sama sekali sampai transaksi kita selesai. Semua
// angka yang keluar dari satu pemanggilan GetSummary ini dijamin berasal
// dari satu titik waktu yang sama persis.
//
// KENAPA endpoint CRUD biasa (create Customer/Job, dst) TIDAK butuh ini:
// mereka cuma satu statement tulis (atau satu transaksi kecil dengan satu
// tujuan atomicity spesifik, mis. invoice.CreateFromJob) - tidak ada
// "banyak pembacaan independen yang harus terlihat sebagai satu snapshot
// koheren buat manusia". Read Committed malah LEBIH BENAR di situ: mereka
// justru ingin melihat data SETERKINI mungkin di tiap langkah (mis. cek
// constraint sebelum insert), bukan snapshot yang dibekukan.
//
// KENAPA read-only transaction ini TIDAK butuh retry logic untuk error
// 40001 (serialization failure): error itu muncul di Postgres kalau
// transaksi men-TULIS baris yang konflik dengan transaksi konkuren lain.
// Transaksi ini TIDAK PERNAH menulis apapun (AccessMode: ReadOnly) - tidak
// ada write-write conflict yang mungkin terjadi, jadi 40001 secara teori
// tidak akan pernah muncul dari transaksi ini. Beda kalau nanti ada modul
// lain yang pakai SERIALIZABLE untuk transaksi yang ADA tulisannya - itu
// baru wajib retry loop.
func (r *sqlcRepository) GetSummary(ctx context.Context, period PeriodInput) (Summary, error) {
	var result Summary

	txOpts := pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	err := pgx.BeginTxFunc(ctx, r.pool, txOpts, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)

		periodFrom := pgconv.ToTimestamptz(period.From)
		periodToExclusive := pgconv.ToTimestamptz(period.ToExclusive)

		invoicedTotal, err := q.InvoicedTotal(ctx, sqlcgen.InvoicedTotalParams{
			PeriodFrom: periodFrom, PeriodToExclusive: periodToExclusive,
		})
		if err != nil {
			return fmt.Errorf("invoiced total: %w", err)
		}

		receivedTotal, err := q.ReceivedTotal(ctx, sqlcgen.ReceivedTotalParams{
			PeriodFrom: periodFrom, PeriodToExclusive: periodToExclusive,
		})
		if err != nil {
			return fmt.Errorf("received total: %w", err)
		}

		outstanding, err := q.OutstandingTotals(ctx)
		if err != nil {
			return fmt.Errorf("outstanding totals: %w", err)
		}

		invoiceStatusRows, err := q.InvoiceCountsByStatus(ctx, sqlcgen.InvoiceCountsByStatusParams{
			PeriodFrom: periodFrom, PeriodToExclusive: periodToExclusive,
		})
		if err != nil {
			return fmt.Errorf("invoice counts by status: %w", err)
		}

		jobStatusRows, err := q.JobCountsByStatus(ctx)
		if err != nil {
			return fmt.Errorf("job counts by status: %w", err)
		}

		upcomingRows, err := q.UpcomingScheduledJobs(ctx)
		if err != nil {
			return fmt.Errorf("upcoming scheduled jobs: %w", err)
		}

		overdueRows, err := q.OverdueScheduledJobs(ctx)
		if err != nil {
			return fmt.Errorf("overdue scheduled jobs: %w", err)
		}

		result = Summary{
			Financial: FinancialSummary{
				Period:        Period{From: period.From, To: period.To},
				InvoicedTotal: pgconv.FromNumeric(invoicedTotal),
				ReceivedTotal: pgconv.FromNumeric(receivedTotal),
				// Sub dilakukan di Go pakai shopspring/decimal (bukan
				// dikurangkan di SQL) - konsisten dengan cara invoice
				// module menghitung nilai turunan (lihat ComputeAmounts),
				// supaya satu-satunya tempat aritmetika uang terjadi di
				// kode yang gampang diuji tanpa Postgres.
				OutstandingTotal: pgconv.FromNumeric(outstanding.Invoiced).Sub(pgconv.FromNumeric(outstanding.Paid)),
				ByStatus:         invoiceStatusBreakdown(invoiceStatusRows),
			},
			Jobs: JobsSummary{
				ByStatus:         jobStatusBreakdown(jobStatusRows),
				Upcoming7Days:    fromUpcomingRows(upcomingRows),
				OverdueScheduled: fromOverdueRows(overdueRows),
			},
		}
		return nil
	})
	if err != nil {
		return Summary{}, err
	}
	return result, nil
}

// invoiceStatusBreakdown mengisi FinancialByStatus dari baris GROUP BY yang
// cuma berisi status yang benar-benar ada datanya di period ini - status
// yang tidak muncul di rows otomatis tetap 0 (zero value Go), bukan hilang
// dari response.
func invoiceStatusBreakdown(rows []sqlcgen.InvoiceCountsByStatusRow) FinancialByStatus {
	var b FinancialByStatus
	for _, row := range rows {
		amount := InvoiceStatusAmount{Count: row.Count, Total: pgconv.FromNumeric(row.Total)}
		switch row.Status {
		case "draft":
			b.Draft = amount
		case "sent":
			b.Sent = amount
		case "paid":
			b.Paid = amount
		case "overdue":
			b.Overdue = amount
		case "cancelled":
			b.Cancelled = amount
		}
	}
	return b
}

func jobStatusBreakdown(rows []sqlcgen.JobCountsByStatusRow) JobsByStatus {
	var b JobsByStatus
	for _, row := range rows {
		switch row.Status {
		case "requested":
			b.Requested = row.Count
		case "scheduled":
			b.Scheduled = row.Count
		case "in_progress":
			b.InProgress = row.Count
		case "completed":
			b.Completed = row.Count
		case "cancelled":
			b.Cancelled = row.Count
		}
	}
	return b
}

func fromUpcomingRows(rows []sqlcgen.UpcomingScheduledJobsRow) []ScheduledJobRef {
	refs := make([]ScheduledJobRef, len(rows))
	for i, row := range rows {
		refs[i] = ScheduledJobRef{
			ID:            pgconv.FromUUID(row.ID),
			JobCode:       row.JobCode,
			CustomerName:  row.CustomerName,
			ScheduledDate: pgconv.FromDate(row.ScheduledDate),
		}
	}
	return refs
}

func fromOverdueRows(rows []sqlcgen.OverdueScheduledJobsRow) []ScheduledJobRef {
	refs := make([]ScheduledJobRef, len(rows))
	for i, row := range rows {
		refs[i] = ScheduledJobRef{
			ID:            pgconv.FromUUID(row.ID),
			JobCode:       row.JobCode,
			CustomerName:  row.CustomerName,
			ScheduledDate: pgconv.FromDate(row.ScheduledDate),
		}
	}
	return refs
}
