package customer

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/nathan/cnc-pm-backend/internal/pgconv"
	"github.com/nathan/cnc-pm-backend/internal/repository/sqlcgen"
)

type MachineRepository interface {
	Create(ctx context.Context, m Machine) (Machine, error)
	ListByCustomer(ctx context.Context, customerID uuid.UUID) ([]Machine, error)
}

type sqlcMachineRepository struct {
	q *sqlcgen.Queries
}

func NewMachineRepository(q *sqlcgen.Queries) MachineRepository {
	return &sqlcMachineRepository{q: q}
}

func (r *sqlcMachineRepository) Create(ctx context.Context, m Machine) (Machine, error) {
	row, err := r.q.CreateMachine(ctx, sqlcgen.CreateMachineParams{
		CustomerID:   pgconv.ToUUID(m.CustomerID),
		MachineName:  m.MachineName,
		MachineType:  pgconv.ToText(m.MachineType),
		SerialNumber: pgconv.ToText(m.SerialNumber),
		Notes:        pgconv.ToText(m.Notes),
	})
	if err != nil {
		return Machine{}, fmt.Errorf("insert machine: %w", err)
	}
	return fromMachineRow(row), nil
}

func (r *sqlcMachineRepository) ListByCustomer(ctx context.Context, customerID uuid.UUID) ([]Machine, error) {
	rows, err := r.q.ListMachinesByCustomer(ctx, pgconv.ToUUID(customerID))
	if err != nil {
		return nil, fmt.Errorf("list machines by customer: %w", err)
	}
	machines := make([]Machine, len(rows))
	for i, row := range rows {
		machines[i] = fromMachineRow(row)
	}
	return machines, nil
}

func fromMachineRow(row sqlcgen.Machine) Machine {
	return Machine{
		ID:           pgconv.FromUUID(row.ID),
		CustomerID:   pgconv.FromUUID(row.CustomerID),
		MachineName:  row.MachineName,
		MachineType:  pgconv.FromText(row.MachineType),
		SerialNumber: pgconv.FromText(row.SerialNumber),
		Notes:        pgconv.FromText(row.Notes),
		CreatedAt:    pgconv.FromTimestamptz(row.CreatedAt),
		UpdatedAt:    pgconv.FromTimestamptz(row.UpdatedAt),
	}
}
