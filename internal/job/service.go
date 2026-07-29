package job

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// CreateInput sengaja tidak punya Status/JobCode - job baru selalu mulai
// dari StatusRequested, dan JobCode di-generate repository.go.
type CreateInput struct {
	CustomerID    uuid.UUID
	MachineID     *uuid.UUID
	Title         string
	Description   *string
	ScheduledDate *time.Time
}

func (s *Service) Create(ctx context.Context, in CreateInput) (Job, error) {
	j := Job{
		CustomerID:    in.CustomerID,
		MachineID:     in.MachineID,
		Title:         in.Title,
		Description:   in.Description,
		Status:        StatusRequested,
		ScheduledDate: in.ScheduledDate,
	}
	if err := j.Validate(); err != nil {
		return Job{}, err
	}

	created, err := s.repo.Create(ctx, j)
	if err != nil {
		return Job{}, fmt.Errorf("create job: %w", err)
	}
	return created, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (Job, error) {
	return s.repo.GetByID(ctx, id)
}

type ListParams struct {
	Status     *JobStatus
	CustomerID *uuid.UUID
	Page       int32
	Limit      int32
}

type ListResult struct {
	Jobs  []Job
	Total int64
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

	filter := ListFilter{Status: p.Status, CustomerID: p.CustomerID}
	jobs, err := s.repo.List(ctx, filter, limit, offset)
	if err != nil {
		return ListResult{}, fmt.Errorf("list jobs: %w", err)
	}
	total, err := s.repo.Count(ctx, filter)
	if err != nil {
		return ListResult{}, fmt.Errorf("count jobs: %w", err)
	}
	return ListResult{Jobs: jobs, Total: total}, nil
}

// UpdateStatus mengubah status job. completed_date otomatis diisi hari ini
// kalau status baru "completed", dan dikosongkan lagi kalau status berubah
// jadi selain completed (mis. job dibuka lagi jadi in_progress) - supaya
// completed_date selalu konsisten dengan status SAAT INI, bukan jejak
// historis "pernah completed kapan". Tidak ada validasi urutan transisi
// (mis. requested -> completed langsung diperbolehkan) - state machine
// yang lebih ketat bisa ditambah nanti kalau memang dibutuhkan bisnisnya.
func (s *Service) UpdateStatus(ctx context.Context, id uuid.UUID, status JobStatus) (Job, error) {
	if !status.Valid() {
		return Job{}, ErrInvalidStatus
	}

	var completedDate *time.Time
	if status == StatusCompleted {
		now := time.Now()
		completedDate = &now
	}

	updated, err := s.repo.UpdateStatus(ctx, id, status, completedDate)
	if err != nil {
		return Job{}, fmt.Errorf("update job status: %w", err)
	}
	return updated, nil
}

func (s *Service) AssignTechnician(ctx context.Context, id, technicianID uuid.UUID) (Job, error) {
	updated, err := s.repo.AssignTechnician(ctx, id, technicianID)
	if err != nil {
		return Job{}, fmt.Errorf("assign technician: %w", err)
	}
	return updated, nil
}
