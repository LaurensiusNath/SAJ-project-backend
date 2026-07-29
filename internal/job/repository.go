package job

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nathan/cnc-pm-backend/internal/pgconv"
	"github.com/nathan/cnc-pm-backend/internal/repository/sqlcgen"
)

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
	UpdateStatus(ctx context.Context, id uuid.UUID, status JobStatus, completedDate *time.Time) (Job, error)
	AssignTechnician(ctx context.Context, id uuid.UUID, technicianID uuid.UUID) (Job, error)
}

type sqlcRepository struct {
	q *sqlcgen.Queries
}

func NewRepository(q *sqlcgen.Queries) Repository {
	return &sqlcRepository{q: q}
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

func (r *sqlcRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status JobStatus, completedDate *time.Time) (Job, error) {
	row, err := r.q.UpdateJobStatus(ctx, sqlcgen.UpdateJobStatusParams{
		ID:            pgconv.ToUUID(id),
		Status:        string(status),
		CompletedDate: pgconv.ToDate(completedDate),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Job{}, ErrNotFound
		}
		return Job{}, fmt.Errorf("update job status: %w", err)
	}
	return fromRow(row), nil
}

func (r *sqlcRepository) AssignTechnician(ctx context.Context, id, technicianID uuid.UUID) (Job, error) {
	row, err := r.q.AssignTechnician(ctx, sqlcgen.AssignTechnicianParams{
		ID:           pgconv.ToUUID(id),
		TechnicianID: pgconv.ToUUID(technicianID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Job{}, ErrNotFound
		}
		return Job{}, fmt.Errorf("assign technician: %w", err)
	}
	return fromRow(row), nil
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
