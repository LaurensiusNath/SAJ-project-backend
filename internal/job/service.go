package job

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/nathan/cnc-pm-backend/internal/customer"
	"github.com/nathan/cnc-pm-backend/internal/notification"
	"github.com/nathan/cnc-pm-backend/internal/user"
)

// Service butuh customer.Repository dan user.Repository selain notifier -
// AssignTechnician/UpdateStatus perlu tahu email customer/teknisi untuk
// mengirim notifikasi. costRepo ditambahkan untuk GetDetail (menyusun
// field `costs` di GET /jobs/{id} tanpa client harus panggil
// GET /jobs/{id}/costs terpisah). Sama pola dependency-nya dengan
// CostService yang bergantung ke job.Repository, cuma di sini menyeberang
// ke modul lain (dan, untuk costRepo, ke tipe lain di package yang sama).
type Service struct {
	repo         Repository
	customerRepo customer.Repository
	userRepo     user.Repository
	costRepo     CostRepository
	notifier     notification.Sender
}

func NewService(repo Repository, customerRepo customer.Repository, userRepo user.Repository, costRepo CostRepository, notifier notification.Sender) *Service {
	return &Service{repo: repo, customerRepo: customerRepo, userRepo: userRepo, costRepo: costRepo, notifier: notifier}
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

// GetDetail dipakai HANYA oleh GET /jobs/{id} (lihat docs/api-contract.md:
// "wajib nested, ini requirement, bukan opsional") - menyusun Job + riwayat
// status + daftar biaya dalam satu response, supaya frontend tidak perlu
// 3x round-trip (GetByID, lalu ListStatusHistory, lalu costRepo.ListByJob
// terpisah).
func (s *Service) GetDetail(ctx context.Context, id uuid.UUID) (Detail, error) {
	j, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Detail{}, err
	}

	history, err := s.repo.ListStatusHistory(ctx, id)
	if err != nil {
		return Detail{}, fmt.Errorf("list job status history: %w", err)
	}

	costs, err := s.costRepo.ListByJob(ctx, id)
	if err != nil {
		return Detail{}, fmt.Errorf("list job costs: %w", err)
	}

	return Detail{Job: j, StatusHistory: history, Costs: costs}, nil
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
//
// changedBy WAJIB diisi (bukan pointer/opsional) - ini identitas user yang
// login, dibaca handler.go dari JWT claim (auth.UserIDFromContext), bukan
// dari body request client (tidak bisa dipalsukan client jadi "atas nama"
// user lain).
func (s *Service) UpdateStatus(ctx context.Context, id uuid.UUID, status JobStatus, changedBy uuid.UUID, notes *string) (Job, error) {
	if !status.Valid() {
		return Job{}, ErrInvalidStatus
	}

	var completedDate *time.Time
	if status == StatusCompleted {
		now := time.Now()
		completedDate = &now
	}

	updated, err := s.repo.UpdateStatus(ctx, id, status, completedDate, changedBy, notes)
	if err != nil {
		return Job{}, fmt.Errorf("update job status: %w", err)
	}

	if status == StatusCompleted {
		s.notifyJobCompleted(ctx, updated)
	}

	return updated, nil
}

// AssignTechnician mensyaratkan expectedUpdatedAt (bukan opsional) - lihat
// repository.go untuk kenapa optimistic locking dipilih di sini. Membuatnya
// opsional akan meniadakan jaminannya sendiri (client yang malas bisa selalu
// melewatinya, race-nya kembali ada diam-diam).
func (s *Service) AssignTechnician(ctx context.Context, id, technicianID uuid.UUID, expectedUpdatedAt time.Time) (Job, error) {
	updated, err := s.repo.AssignTechnician(ctx, id, technicianID, expectedUpdatedAt)
	if err != nil {
		return Job{}, fmt.Errorf("assign technician: %w", err)
	}

	s.notifyAssignment(ctx, updated, technicianID)

	return updated, nil
}

// notifyAssignment mengirim email ke customer (kalau punya email tercatat)
// dan ke teknisi yang baru ditugaskan - best-effort, TIDAK PERNAH
// menggagalkan AssignTechnician sendiri kalau pengiriman gagal (lihat
// notification.Service.SendEmail). Kegagalan cukup di-log lewat package
// "log" standar, bukan dikembalikan sebagai error - beda sengaja dari pola
// error lain di file ini, karena tidak ada tindakan yang bisa/perlu
// dilakukan caller HTTP soal gagalnya notifikasi (request assign-nya sendiri
// tetap sukses).
func (s *Service) notifyAssignment(ctx context.Context, j Job, technicianID uuid.UUID) {
	if cust, err := s.customerRepo.GetByID(ctx, j.CustomerID); err == nil && cust.Email != nil {
		body := fmt.Sprintf("Teknisi telah ditugaskan untuk servis %q (kode job %s).", j.Title, j.JobCode)
		if err := s.notifier.SendEmail(ctx, notification.EmailInput{
			To: *cust.Email, Subject: "Teknisi Ditugaskan untuk Servis Anda", Body: body, JobID: &j.ID,
		}); err != nil {
			log.Printf("notifikasi assign ke customer %s gagal: %v", *cust.Email, err)
		}
	}

	if tech, err := s.userRepo.GetByID(ctx, technicianID); err == nil {
		body := fmt.Sprintf("Anda ditugaskan untuk job %s (%s).", j.JobCode, j.Title)
		if err := s.notifier.SendEmail(ctx, notification.EmailInput{
			To: tech.Email, Subject: "Penugasan Job Baru", Body: body, JobID: &j.ID,
		}); err != nil {
			log.Printf("notifikasi assign ke teknisi %s gagal: %v", tech.Email, err)
		}
	}
}

// notifyJobCompleted sama sifatnya (best-effort) dengan notifyAssignment.
func (s *Service) notifyJobCompleted(ctx context.Context, j Job) {
	cust, err := s.customerRepo.GetByID(ctx, j.CustomerID)
	if err != nil || cust.Email == nil {
		return
	}
	body := fmt.Sprintf("Servis untuk job %s (%s) telah selesai.", j.JobCode, j.Title)
	if err := s.notifier.SendEmail(ctx, notification.EmailInput{
		To: *cust.Email, Subject: "Servis Anda Telah Selesai", Body: body, JobID: &j.ID,
	}); err != nil {
		log.Printf("notifikasi job completed ke customer %s gagal: %v", *cust.Email, err)
	}
}
