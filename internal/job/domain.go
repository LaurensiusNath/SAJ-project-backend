package job

import (
	"errors"
	"time"

	"github.com/google/uuid"
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
	ErrNotFound      = errors.New("job not found")
	ErrInvalidTitle  = errors.New("job title is required")
	ErrInvalidStatus = errors.New("status must be one of: requested, scheduled, in_progress, completed, cancelled")
)

// Job adalah entity domain untuk satu work order. MachineID, TechnicianID,
// ScheduledDate, CompletedDate memakai pointer karena semuanya nullable di
// kolom DB (job bisa dibuat sebelum mesin/teknisi ditentukan).
//
// JobCode sengaja tidak ada di CreateInput (lihat service.go) - di-generate
// backend saat Create, sama seperti ID/CreatedAt, bukan input dari client.
type Job struct {
	ID            uuid.UUID  `json:"id"`
	JobCode       string     `json:"job_code"`
	CustomerID    uuid.UUID  `json:"customer_id"`
	MachineID     *uuid.UUID `json:"machine_id"`
	TechnicianID  *uuid.UUID `json:"technician_id"`
	Title         string     `json:"title"`
	Description   *string    `json:"description"`
	Status        JobStatus  `json:"status"`
	ScheduledDate *time.Time `json:"scheduled_date"`
	CompletedDate *time.Time `json:"completed_date"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (j Job) Validate() error {
	if j.Title == "" {
		return ErrInvalidTitle
	}
	if !j.Status.Valid() {
		return ErrInvalidStatus
	}
	return nil
}
