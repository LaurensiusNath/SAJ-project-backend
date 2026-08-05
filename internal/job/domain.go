package job

import (
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/nathan/cnc-pm-backend/internal/dateonly"
)

// JobStatus merepresentasikan kolom jobs.status - pola typed-string enum
// yang sama seperti CustomerType di modul customer.
type JobStatus string

const (
	StatusRequested  JobStatus = "requested"
	StatusScheduled  JobStatus = "scheduled"
	StatusInProgress JobStatus = "in_progress"
	StatusCompleted  JobStatus = "completed"
	StatusCancelled  JobStatus = "cancelled"
)

func (s JobStatus) Valid() bool {
	switch s {
	case StatusRequested, StatusScheduled, StatusInProgress, StatusCompleted, StatusCancelled:
		return true
	default:
		return false
	}
}

var (
	ErrNotFound        = errors.New("job not found")
	ErrInvalidTitle    = errors.New("job title is required")
	ErrInvalidStatus   = errors.New("status must be one of: requested, scheduled, in_progress, completed, cancelled")
	ErrInvalidCustomer = errors.New("customer_id is required")
	// ErrInvalidReference: customer_id/machine_id/technician_id menunjuk ke
	// baris yang tidak ada di tabel referensinya (foreign key violation).
	ErrInvalidReference = errors.New("referenced record does not exist")
	// ErrConflict: optimistic locking gagal - job ini sudah diubah pihak lain
	// sejak client terakhir membaca updated_at-nya (lihat AssignTechnician
	// di repository.go untuk kenapa ini dipakai, bukan pessimistic lock).
	ErrConflict = errors.New("job was modified by someone else - refetch and retry with the latest updated_at")
)

// Job adalah entity domain untuk satu work order. MachineID, TechnicianID,
// ScheduledDate, CompletedDate memakai pointer karena semuanya nullable di
// kolom DB (job bisa dibuat sebelum mesin/teknisi ditentukan).
//
// JobCode sengaja tidak ada di CreateInput (lihat service.go) - di-generate
// backend saat Create, sama seperti ID/CreatedAt, bukan input dari client.
type Job struct {
	ID            uuid.UUID      `json:"id"`
	JobCode       string         `json:"job_code"`
	CustomerID    uuid.UUID      `json:"customer_id"`
	MachineID     *uuid.UUID     `json:"machine_id"`
	TechnicianID  *uuid.UUID     `json:"technician_id"`
	Title         string         `json:"title"`
	Description   *string        `json:"description"`
	Status        JobStatus      `json:"status"`
	ScheduledDate *dateonly.Date `json:"scheduled_date"`
	CompletedDate *dateonly.Date `json:"completed_date"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

func (j Job) Validate() error {
	if j.CustomerID == uuid.Nil {
		return ErrInvalidCustomer
	}
	if j.Title == "" {
		return ErrInvalidTitle
	}
	if !j.Status.Valid() {
		return ErrInvalidStatus
	}
	return nil
}

// JobStatusHistory adalah satu baris audit trail perubahan status - selalu
// ditulis bersamaan (dalam satu transaksi) dengan UPDATE jobs.status oleh
// repository.go, tidak pernah ditulis manual dari tempat lain. ChangedBy
// selalu terisi (bukan pointer) karena satu-satunya jalur penulisan
// (PATCH /jobs/{id}/status) selalu melewati RequireAuth.
type JobStatusHistory struct {
	ID        uuid.UUID `json:"id"`
	JobID     uuid.UUID `json:"job_id"`
	Status    JobStatus `json:"status"`
	ChangedBy uuid.UUID `json:"changed_by"`
	ChangedAt time.Time `json:"changed_at"`
	Notes     *string   `json:"notes"`
}

// Detail membungkus Job + status_history + costs, sesuai
// docs/api-contract.md ("GET /jobs/{id} wajib nested") - dipakai HANYA
// oleh Handler.GetByID, bukan endpoint lain (List/Create/dst tetap
// mengembalikan Job polos, sama seperti customer.customerDetailResponse
// yang cuma dipakai di GetByID).
type Detail struct {
	Job
	StatusHistory []JobStatusHistory `json:"status_history"`
	Costs         []JobCost          `json:"costs"`
}
