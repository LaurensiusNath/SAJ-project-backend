package invoice

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/nathan/cnc-pm-backend/internal/customer"
	"github.com/nathan/cnc-pm-backend/internal/job"
	"github.com/nathan/cnc-pm-backend/internal/pgconv"
	"github.com/nathan/cnc-pm-backend/internal/repository/sqlcgen"
	"github.com/nathan/cnc-pm-backend/internal/settings"
)

const uniqueViolation = "23505"

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

type ListFilter struct {
	Status *Status
}

type CreateFromJobInput struct {
	JobID uuid.UUID
	// TaxPercentage nil berarti pakai company_settings.default_tax_percentage
	// (lihat resolusinya di CreateFromJob).
	TaxPercentage *decimal.Decimal
	DueDate       *time.Time
}

type RecordPaymentInput struct {
	InvoiceID           uuid.UUID
	Amount              decimal.Decimal
	PaymentMethod       PaymentMethod
	BuktiPotongPPh23Ref *string
	Notes               *string
}

type Repository interface {
	// CreateFromJob membungkus SEMUA langkah generate invoice dalam satu
	// transaksi: mengunci baris job (GetJobForUpdate), memastikan job
	// completed dan belum punya invoice, membaca job_costs + customer +
	// company_settings, menghitung, lalu insert. Lihat penjelasan lengkap
	// di badan fungsi soal kenapa locking di sini penting (Atomicity +
	// mencegah race check-then-act).
	CreateFromJob(ctx context.Context, in CreateFromJobInput) (Invoice, error)
	GetByID(ctx context.Context, id uuid.UUID) (Invoice, error)
	List(ctx context.Context, filter ListFilter, limit, offset int32) ([]Invoice, error)
	Count(ctx context.Context, filter ListFilter) (int64, error)
	UpdateFakturPajak(ctx context.Context, id uuid.UUID, nomorFakturPajak string) (Invoice, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status Status) (Invoice, error)
	// RecordPayment mengunci baris invoice (GetInvoiceForUpdate) sebelum
	// insert payment dan menghitung ulang status - lihat badan fungsi untuk
	// penjelasan lost update yang dicegah lock ini.
	RecordPayment(ctx context.Context, in RecordPaymentInput) (Payment, error)
	ListPayments(ctx context.Context, invoiceID uuid.UUID) ([]Payment, error)
}

type sqlcRepository struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

// NewRepository butuh *pgxpool.Pool selain *sqlcgen.Queries - beda dari
// repository modul lain (customer/job/settings) yang cukup Queries saja.
// Alasannya: CreateFromJob dan RecordPayment perlu membuka transaksi
// (pool.Begin), lalu membuat instance Queries versi tx (q.WithTx(tx)) yang
// dipakai untuk semua statement di dalam transaksi itu. Modul lain belum
// butuh ini karena belum ada operasi mereka yang menyentuh lebih dari satu
// tabel sekaligus secara atomik.
func NewRepository(pool *pgxpool.Pool, q *sqlcgen.Queries) Repository {
	return &sqlcRepository{pool: pool, q: q}
}

// CreateFromJob adalah contoh Atomicity yang diminta: banyak pembacaan
// (job, job_costs, customer, company_settings) + satu tulisan (insert
// invoice) harus terjadi seolah satu langkah - kalau salah satu gagal di
// tengah, semuanya batal (Rollback otomatis lewat pgx.BeginFunc).
//
// Tanpa mengunci baris job, ada race condition check-then-act: dua request
// "generate invoice" untuk job yang sama bisa sama-sama lolos pengecekan
// "belum ada invoice" sebelum salah satunya sempat insert - keduanya
// pikir mereka yang pertama. GetJobForUpdate (SELECT ... FOR UPDATE)
// menutup celah ini: request kedua akan menunggu di baris itu sampai
// request pertama commit, baru pengecekan "sudah ada invoice belum"-nya
// melihat data yang benar-benar terbaru. UNIQUE constraint di
// invoices.job_id tetap ada sebagai pagar terakhir kalau suatu saat kode
// ini dipanggil tanpa lewat lock ini (mis. dari migrasi data manual).
//
// Isolation level pakai default Postgres (Read Committed) - cukup di sini
// karena kita mengandalkan row lock eksplisit (FOR UPDATE), bukan
// mengandalkan snapshot transaksi untuk konsistensi. Kalau ada modul lain
// nanti yang butuh "melihat banyak tabel sebagai satu snapshot yang
// konsisten" (mis. Dashboard), itu baru kandidat Repeatable Read/Serializable.
func (r *sqlcRepository) CreateFromJob(ctx context.Context, in CreateFromJobInput) (Invoice, error) {
	var result Invoice

	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)

		if _, err := q.GetJobForUpdate(ctx, pgconv.ToUUID(in.JobID)); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return job.ErrNotFound
			}
			return fmt.Errorf("lock job row: %w", err)
		}

		jobRepo := job.NewRepository(q)
		j, err := jobRepo.GetByID(ctx, in.JobID)
		if err != nil {
			return err
		}
		if j.Status != job.StatusCompleted {
			return ErrJobNotCompleted
		}

		if _, err := q.GetInvoiceByJobID(ctx, pgconv.ToUUID(in.JobID)); err == nil {
			return ErrAlreadyInvoiced
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("check existing invoice: %w", err)
		}

		costTotals, err := job.NewCostRepository(q).InvoiceTotals(ctx, in.JobID)
		if err != nil {
			return err
		}

		cust, err := customer.NewRepository(q).GetByID(ctx, j.CustomerID)
		if err != nil {
			return fmt.Errorf("get customer: %w", err)
		}

		companySettings, err := settings.NewRepository(q).Get(ctx)
		if err != nil {
			return fmt.Errorf("get company settings: %w", err)
		}

		// tax_percentage boleh di-override per invoice (kasus khusus) - kalau
		// client tidak mengirimnya, pakai default dari company_settings supaya
		// perubahan tarif PPN cukup diubah di satu tempat.
		taxPercentage := companySettings.DefaultTaxPercentage
		if in.TaxPercentage != nil {
			taxPercentage = *in.TaxPercentage
		}

		amounts := ComputeAmounts(
			costTotals.SubtotalAll,
			costTotals.DPPPPh23,
			taxPercentage,
			companySettings.DefaultPPh23Rate,
			cust.CustomerType == customer.CustomerTypeBadanUsaha,
		)

		invoiceNumber, err := nextInvoiceNumber(ctx, q)
		if err != nil {
			return err
		}

		row, err := q.CreateInvoice(ctx, sqlcgen.CreateInvoiceParams{
			InvoiceNumber:        invoiceNumber,
			JobID:                pgconv.ToUUID(in.JobID),
			Subtotal:             pgconv.ToNumeric(amounts.Subtotal),
			TaxPercentage:        pgconv.ToNumeric(taxPercentage),
			TaxAmount:            pgconv.ToNumeric(amounts.TaxAmount),
			Total:                pgconv.ToNumeric(amounts.Total),
			DppPph23:             pgconv.ToNumeric(amounts.DPPPPh23),
			Pph23Rate:            pgconv.ToNumeric(companySettings.DefaultPPh23Rate),
			Pph23EstimatedAmount: pgconv.ToNumeric(amounts.PPh23EstimatedAmount),
			ExpectedReceivable:   pgconv.ToNumeric(amounts.ExpectedReceivable),
			DueDate:              pgconv.ToDate(in.DueDate),
		})
		if err != nil {
			if isUniqueViolation(err) {
				return ErrAlreadyInvoiced
			}
			return fmt.Errorf("insert invoice: %w", err)
		}

		result = fromInvoiceRow(row)
		return nil
	})
	if err != nil {
		return Invoice{}, err
	}
	return result, nil
}

// nextInvoiceNumber generate INV-<tahun>-<urutan>, pola dan celah race
// condition kecil yang sama dengan job.sqlcRepository.Create/job_code
// (diterima di skala project ini - lihat repository.go modul job).
func nextInvoiceNumber(ctx context.Context, q *sqlcgen.Queries) (string, error) {
	year := int32(time.Now().Year())
	count, err := q.CountInvoicesByYear(ctx, year)
	if err != nil {
		return "", fmt.Errorf("count invoices by year: %w", err)
	}
	return fmt.Sprintf("INV-%d-%04d", year, count+1), nil
}

func (r *sqlcRepository) GetByID(ctx context.Context, id uuid.UUID) (Invoice, error) {
	row, err := r.q.GetInvoiceByID(ctx, pgconv.ToUUID(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Invoice{}, ErrNotFound
		}
		return Invoice{}, fmt.Errorf("get invoice by id: %w", err)
	}
	return fromInvoiceRow(row), nil
}

func (r *sqlcRepository) List(ctx context.Context, filter ListFilter, limit, offset int32) ([]Invoice, error) {
	rows, err := r.q.ListInvoices(ctx, sqlcgen.ListInvoicesParams{
		Limit:  limit,
		Offset: offset,
		Status: toPgTextFromStatus(filter.Status),
	})
	if err != nil {
		return nil, fmt.Errorf("list invoices: %w", err)
	}
	invoices := make([]Invoice, len(rows))
	for i, row := range rows {
		invoices[i] = fromInvoiceRow(row)
	}
	return invoices, nil
}

func (r *sqlcRepository) Count(ctx context.Context, filter ListFilter) (int64, error) {
	total, err := r.q.CountInvoices(ctx, toPgTextFromStatus(filter.Status))
	if err != nil {
		return 0, fmt.Errorf("count invoices: %w", err)
	}
	return total, nil
}

func (r *sqlcRepository) UpdateFakturPajak(ctx context.Context, id uuid.UUID, nomorFakturPajak string) (Invoice, error) {
	row, err := r.q.UpdateInvoiceFakturPajak(ctx, sqlcgen.UpdateInvoiceFakturPajakParams{
		ID:               pgconv.ToUUID(id),
		NomorFakturPajak: pgconv.ToText(&nomorFakturPajak),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Invoice{}, ErrNotFound
		}
		return Invoice{}, fmt.Errorf("update faktur pajak: %w", err)
	}
	return fromInvoiceRow(row), nil
}

func (r *sqlcRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status Status) (Invoice, error) {
	row, err := r.q.UpdateInvoiceStatus(ctx, sqlcgen.UpdateInvoiceStatusParams{
		ID:     pgconv.ToUUID(id),
		Status: string(status),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Invoice{}, ErrNotFound
		}
		return Invoice{}, fmt.Errorf("update invoice status: %w", err)
	}
	return fromInvoiceRow(row), nil
}

// RecordPayment adalah contoh lost update yang diminta: tanpa mengunci
// baris invoice, dua pembayaran yang masuk nyaris bersamaan bisa
// sama-sama menghitung SUM(payments) dari data yang belum melihat
// pembayaran satu sama lain (masing-masing transaksi hanya melihat commit
// yang sudah selesai duluan, itu perilaku normal Read Committed). Akibatnya
// kombinasi dua pembayaran yang seharusnya melunasi invoice, malah
// membuat status tetap "sent" selamanya - padahal uangnya sudah lengkap
// diterima.
//
// GetInvoiceForUpdate (SELECT ... FOR UPDATE) menutup celah ini: transaksi
// kedua wajib menunggu transaksi pertama commit dulu sebelum dia sendiri
// bisa lanjut - jadi saat dia menghitung ulang SUM(payments), pembayaran
// dari transaksi pertama sudah pasti ikut terhitung.
func (r *sqlcRepository) RecordPayment(ctx context.Context, in RecordPaymentInput) (Payment, error) {
	var result Payment

	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)

		invRow, err := q.GetInvoiceForUpdate(ctx, pgconv.ToUUID(in.InvoiceID))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("lock invoice row: %w", err)
		}
		inv := fromInvoiceRow(invRow)
		if inv.Status != StatusDraft && inv.Status != StatusSent {
			return ErrInvoiceNotPayable
		}

		paymentRow, err := q.CreatePayment(ctx, sqlcgen.CreatePaymentParams{
			InvoiceID:           pgconv.ToUUID(in.InvoiceID),
			Amount:              pgconv.ToNumeric(in.Amount),
			PaymentMethod:       string(in.PaymentMethod),
			BuktiPotongPph23Ref: pgconv.ToText(in.BuktiPotongPPh23Ref),
			Notes:               pgconv.ToText(in.Notes),
		})
		if err != nil {
			return fmt.Errorf("insert payment: %w", err)
		}

		totals, err := q.PaymentTotalsByInvoice(ctx, pgconv.ToUUID(in.InvoiceID))
		if err != nil {
			return fmt.Errorf("payment totals: %w", err)
		}
		totalPaid := pgconv.FromNumeric(totals.TotalPaid)

		// api-contract.md: invoice lunas kalau SUM(payments.amount) + SUM
		// pph23 yang tercatat via bukti potong >= total. pph23_estimated_amount
		// cuma satu nilai per invoice (bukan per payment), jadi diterjemahkan
		// sebagai: ditambahkan SEKALI kalau ADA payment yang mencatat
		// bukti_potong_pph23_ref, bukan dijumlah berkali-kali per payment.
		effectivePaid := totalPaid
		if totals.HasBuktiPotong {
			effectivePaid = effectivePaid.Add(inv.PPh23EstimatedAmount)
		}

		if effectivePaid.GreaterThanOrEqual(inv.Total) {
			if _, err := q.UpdateInvoiceStatus(ctx, sqlcgen.UpdateInvoiceStatusParams{
				ID:     pgconv.ToUUID(in.InvoiceID),
				Status: string(StatusPaid),
			}); err != nil {
				return fmt.Errorf("mark invoice paid: %w", err)
			}
		}

		result = fromPaymentRow(paymentRow)
		return nil
	})
	if err != nil {
		return Payment{}, err
	}
	return result, nil
}

func (r *sqlcRepository) ListPayments(ctx context.Context, invoiceID uuid.UUID) ([]Payment, error) {
	rows, err := r.q.ListPaymentsByInvoice(ctx, pgconv.ToUUID(invoiceID))
	if err != nil {
		return nil, fmt.Errorf("list payments: %w", err)
	}
	payments := make([]Payment, len(rows))
	for i, row := range rows {
		payments[i] = fromPaymentRow(row)
	}
	return payments, nil
}

func toPgTextFromStatus(s *Status) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: string(*s), Valid: true}
}

func fromInvoiceRow(row sqlcgen.Invoice) Invoice {
	return Invoice{
		ID:                   pgconv.FromUUID(row.ID),
		InvoiceNumber:        row.InvoiceNumber,
		NomorFakturPajak:     pgconv.FromText(row.NomorFakturPajak),
		JobID:                pgconv.FromUUID(row.JobID),
		Subtotal:             pgconv.FromNumeric(row.Subtotal),
		TaxPercentage:        pgconv.FromNumeric(row.TaxPercentage),
		TaxAmount:            pgconv.FromNumeric(row.TaxAmount),
		Total:                pgconv.FromNumeric(row.Total),
		DPPPPh23:             pgconv.FromNumeric(row.DppPph23),
		PPh23Rate:            pgconv.FromNumeric(row.Pph23Rate),
		PPh23EstimatedAmount: pgconv.FromNumeric(row.Pph23EstimatedAmount),
		ExpectedReceivable:   pgconv.FromNumeric(row.ExpectedReceivable),
		Status:               Status(row.Status),
		DueDate:              pgconv.FromDate(row.DueDate),
		CreatedAt:            pgconv.FromTimestamptz(row.CreatedAt),
		UpdatedAt:            pgconv.FromTimestamptz(row.UpdatedAt),
	}
}

func fromPaymentRow(row sqlcgen.Payment) Payment {
	return Payment{
		ID:                  pgconv.FromUUID(row.ID),
		InvoiceID:           pgconv.FromUUID(row.InvoiceID),
		Amount:              pgconv.FromNumeric(row.Amount),
		PaymentMethod:       PaymentMethod(row.PaymentMethod),
		BuktiPotongPPh23Ref: pgconv.FromText(row.BuktiPotongPph23Ref),
		Notes:               pgconv.FromText(row.Notes),
		CreatedAt:           pgconv.FromTimestamptz(row.CreatedAt),
	}
}
