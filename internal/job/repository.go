package job

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

	"github.com/nathan/cnc-pm-backend/internal/pgconv"
	"github.com/nathan/cnc-pm-backend/internal/repository/sqlcgen"
)

// foreignKeyViolation is Postgres' error code for a violated FK constraint
// (https://www.postgresql.org/docs/current/errcodes-appendix.html). Mapped
// to the domain's ErrInvalidReference so the handler can return 400 instead
// of leaking a raw DB error as a 500.
const foreignKeyViolation = "23503"

func asInvalidReference(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation {
		return fmt.Errorf("%w: %s", ErrInvalidReference, pgErr.ConstraintName)
	}
	return nil
}

// ListFilter dipakai bersama oleh List dan Count, sama pola dengan
// customer.ListFilter - supaya meta.total tidak bisa "berbeda kriteria"
// dari data yang benar-benar ditampilkan.
type ListFilter struct {
	Status     *JobStatus
	CustomerID *uuid.UUID
}

type Repository interface {
	Create(ctx context.Context, j Job) (Job, error)
	GetByID(ctx context.Context, id uuid.UUID) (Job, error)
	List(ctx context.Context, filter ListFilter, limit, offset int32) ([]Job, error)
	Count(ctx context.Context, filter ListFilter) (int64, error)
	// UpdateStatus menulis UPDATE jobs.status DAN INSERT job_status_history
	// dalam SATU transaksi (lihat badan fungsi) - changedBy datang dari JWT
	// claim user yang login (dibaca handler.go lewat auth.UserIDFromContext),
	// bukan dari body request.
	UpdateStatus(ctx context.Context, id uuid.UUID, status JobStatus, completedDate *time.Time, changedBy uuid.UUID, notes *string) (Job, error)
	// ListStatusHistory dipakai Service.GetDetail untuk menyusun field
	// status_history di response GET /jobs/{id}.
	ListStatusHistory(ctx context.Context, jobID uuid.UUID) ([]JobStatusHistory, error)
	// AssignTechnician pakai optimistic locking: expectedUpdatedAt harus
	// persis sama dengan jobs.updated_at saat ini, kalau tidak (sudah diubah
	// pihak lain) mengembalikan ErrConflict. Lihat penjelasan lengkap di
	// badan fungsi.
	AssignTechnician(ctx context.Context, id uuid.UUID, technicianID uuid.UUID, expectedUpdatedAt time.Time) (Job, error)
	// ListNeedingReminderCheck dipakai job.ReminderService - job yang
	// jadwalnya sudah lewat, hari ini, atau besok, dan belum completed/cancelled.
	ListNeedingReminderCheck(ctx context.Context) ([]Job, error)
}

type sqlcRepository struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

// NewRepository sekarang butuh *pgxpool.Pool juga (sebelumnya cukup
// Queries) - UpdateStatus perlu membuka transaksi sendiri supaya UPDATE
// jobs.status dan INSERT job_status_history konsisten (lihat badan fungsi
// UpdateStatus). Pola yang sama seperti invoice.NewRepository.
func NewRepository(pool *pgxpool.Pool, q *sqlcgen.Queries) Repository {
	return &sqlcRepository{pool: pool, q: q}
}

// Create men-generate job_code sendiri (JOB-<tahun>-<urutan>) dari hitungan
// job tahun berjalan - makanya JobCode tidak ada di CreateInput service.go,
// persis seperti ID/CreatedAt yang tidak pernah diterima dari client.
//
// Catatan: ada celah race condition kecil antara CountJobsByYear dan
// CreateJob (dua request bersamaan bisa dapat urutan yang sama) - untuk
// skala satu bengkel CNC ini diterima sebagai simplifikasi; kalau volume
// job makin tinggi, ganti ke DB sequence per tahun.
func (r *sqlcRepository) Create(ctx context.Context, j Job) (Job, error) {
	year := int32(time.Now().Year())
	count, err := r.q.CountJobsByYear(ctx, year)
	if err != nil {
		return Job{}, fmt.Errorf("count jobs by year: %w", err)
	}
	jobCode := fmt.Sprintf("JOB-%d-%04d", year, count+1)

	row, err := r.q.CreateJob(ctx, sqlcgen.CreateJobParams{
		JobCode:       jobCode,
		CustomerID:    pgconv.ToUUID(j.CustomerID),
		MachineID:     pgconv.ToNullableUUID(j.MachineID),
		Title:         j.Title,
		Description:   pgconv.ToText(j.Description),
		ScheduledDate: pgconv.ToDate(j.ScheduledDate),
	})
	if err != nil {
		if mapped := asInvalidReference(err); mapped != nil {
			return Job{}, mapped
		}
		return Job{}, fmt.Errorf("insert job: %w", err)
	}
	return fromRow(row), nil
}

func (r *sqlcRepository) GetByID(ctx context.Context, id uuid.UUID) (Job, error) {
	row, err := r.q.GetJobByID(ctx, pgconv.ToUUID(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Job{}, ErrNotFound
		}
		return Job{}, fmt.Errorf("get job by id: %w", err)
	}
	return fromRow(row), nil
}

func (r *sqlcRepository) List(ctx context.Context, filter ListFilter, limit, offset int32) ([]Job, error) {
	rows, err := r.q.ListJobs(ctx, sqlcgen.ListJobsParams{
		Limit:      limit,
		Offset:     offset,
		Status:     toPgTextFromStatus(filter.Status),
		CustomerID: pgconv.ToNullableUUID(filter.CustomerID),
	})
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	jobs := make([]Job, len(rows))
	for i, row := range rows {
		jobs[i] = fromRow(row)
	}
	return jobs, nil
}

func (r *sqlcRepository) Count(ctx context.Context, filter ListFilter) (int64, error) {
	total, err := r.q.CountJobs(ctx, sqlcgen.CountJobsParams{
		Status:     toPgTextFromStatus(filter.Status),
		CustomerID: pgconv.ToNullableUUID(filter.CustomerID),
	})
	if err != nil {
		return 0, fmt.Errorf("count jobs: %w", err)
	}
	return total, nil
}

// UpdateStatus membungkus UPDATE jobs + INSERT job_status_history dalam
// satu transaksi (pgx.BeginFunc) - kalau salah satu gagal, keduanya batal.
// Tanpa ini, ada risiko status jobs berubah tapi baris histori gagal
// tercatat (atau sebaliknya), membuat audit trail tidak bisa dipercaya -
// persis alasan yang sama dengan kenapa invoice.CreateFromJob dibungkus
// transaksi (Atomicity).
func (r *sqlcRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status JobStatus, completedDate *time.Time, changedBy uuid.UUID, notes *string) (Job, error) {
	var result Job

	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)

		row, err := q.UpdateJobStatus(ctx, sqlcgen.UpdateJobStatusParams{
			ID:            pgconv.ToUUID(id),
			Status:        string(status),
			CompletedDate: pgconv.ToDate(completedDate),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("update job status: %w", err)
		}

		if _, err := q.CreateJobStatusHistory(ctx, sqlcgen.CreateJobStatusHistoryParams{
			JobID:     pgconv.ToUUID(id),
			Status:    string(status),
			ChangedBy: pgconv.ToUUID(changedBy),
			Notes:     pgconv.ToText(notes),
		}); err != nil {
			return fmt.Errorf("insert job status history: %w", err)
		}

		result = fromRow(row)
		return nil
	})
	if err != nil {
		return Job{}, err
	}
	return result, nil
}

func (r *sqlcRepository) ListStatusHistory(ctx context.Context, jobID uuid.UUID) ([]JobStatusHistory, error) {
	rows, err := r.q.ListJobStatusHistory(ctx, pgconv.ToUUID(jobID))
	if err != nil {
		return nil, fmt.Errorf("list job status history: %w", err)
	}
	history := make([]JobStatusHistory, len(rows))
	for i, row := range rows {
		history[i] = JobStatusHistory{
			ID:        pgconv.FromUUID(row.ID),
			JobID:     pgconv.FromUUID(row.JobID),
			Status:    JobStatus(row.Status),
			ChangedBy: pgconv.FromUUID(row.ChangedBy),
			ChangedAt: pgconv.FromTimestamptz(row.ChangedAt),
			Notes:     pgconv.FromText(row.Notes),
		}
	}
	return history, nil
}

// AssignTechnician: dua admin yang assign teknisi berbeda ke job yang sama
// nyaris bersamaan tidak akan crash atau deadlock - keduanya sama-sama
// "berhasil" secara SQL (last write wins secara alami). Masalahnya justru
// itu: TIDAK ADA yang tahu tulisannya baru saja ditimpa pihak lain (silent
// overwrite), beda dari kasus payment yang butuh SUM ulang dari data yang
// konsisten.
//
// Pessimistic lock (SELECT ... FOR UPDATE, seperti di invoice.RecordPayment)
// TIDAK menyelesaikan masalah ini - dia cuma menyerialkan urutan tulis,
// request kedua tetap menimpa nilai request pertama begitu lock dilepas,
// cuma bergiliran, bukan barengan. Yang benar-benar dibutuhkan adalah
// optimistic locking: client wajib mengirim updated_at yang terakhir dia
// baca (expectedUpdatedAt); kalau job sudah berubah sejak itu, UPDATE ini
// mengenai 0 baris dan kita kembalikan ErrConflict (409) - klien tahu ada
// konflik, bisa refetch dan putuskan sendiri (retry/timpa/batal), bukan
// diam-diam kalah.
func (r *sqlcRepository) AssignTechnician(ctx context.Context, id, technicianID uuid.UUID, expectedUpdatedAt time.Time) (Job, error) {
	row, err := r.q.AssignTechnician(ctx, sqlcgen.AssignTechnicianParams{
		ID:           pgconv.ToUUID(id),
		TechnicianID: pgconv.ToUUID(technicianID),
		UpdatedAt:    pgconv.ToTimestamptz(expectedUpdatedAt),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// 0 baris ter-update bisa berarti dua hal: job memang tidak ada,
			// atau job ada tapi updated_at-nya sudah berubah (konflik). Perlu
			// satu SELECT tambahan untuk membedakan keduanya.
			if _, getErr := r.GetByID(ctx, id); getErr != nil {
				return Job{}, getErr
			}
			return Job{}, ErrConflict
		}
		if mapped := asInvalidReference(err); mapped != nil {
			return Job{}, mapped
		}
		return Job{}, fmt.Errorf("assign technician: %w", err)
	}
	return fromRow(row), nil
}

func (r *sqlcRepository) ListNeedingReminderCheck(ctx context.Context) ([]Job, error) {
	rows, err := r.q.ListJobsNeedingReminderCheck(ctx)
	if err != nil {
		return nil, fmt.Errorf("list jobs needing reminder check: %w", err)
	}
	jobs := make([]Job, len(rows))
	for i, row := range rows {
		jobs[i] = fromRow(row)
	}
	return jobs, nil
}

// toPgTextFromStatus tetap khusus di sini (bukan masuk pgconv), sama alasan
// dengan toPgTextFromCustomerType di modul customer - terikat tipe JobStatus.
func toPgTextFromStatus(s *JobStatus) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: string(*s), Valid: true}
}

func fromRow(row sqlcgen.Job) Job {
	return Job{
		ID:            pgconv.FromUUID(row.ID),
		JobCode:       row.JobCode,
		CustomerID:    pgconv.FromUUID(row.CustomerID),
		MachineID:     pgconv.FromNullableUUID(row.MachineID),
		TechnicianID:  pgconv.FromNullableUUID(row.TechnicianID),
		Title:         row.Title,
		Description:   pgconv.FromText(row.Description),
		Status:        JobStatus(row.Status),
		ScheduledDate: pgconv.FromDate(row.ScheduledDate),
		CompletedDate: pgconv.FromDate(row.CompletedDate),
		CreatedAt:     pgconv.FromTimestamptz(row.CreatedAt),
		UpdatedAt:     pgconv.FromTimestamptz(row.UpdatedAt),
	}
}
