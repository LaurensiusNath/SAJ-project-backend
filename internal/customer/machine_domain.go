package customer

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidMachineName = errors.New("machine_name is required")
)

// Machine adalah entity domain untuk satu mesin CNC milik sebuah Customer -
// sub-resource, bukan modul sendiri (mirip JobCost di modul job), makanya
// hidup di package customer, bukan package machine terpisah.
type Machine struct {
	ID           uuid.UUID `json:"id"`
	CustomerID   uuid.UUID `json:"customer_id"`
	MachineName  string    `json:"machine_name"`
	MachineType  *string   `json:"machine_type"`
	SerialNumber *string   `json:"serial_number"`
	Notes        *string   `json:"notes"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (m Machine) Validate() error {
	if m.MachineName == "" {
		return ErrInvalidMachineName
	}
	return nil
}
