package customer

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// MachineService butuh Repository (customer) selain MachineRepository sendiri -
// supaya bisa memastikan customer-nya benar-benar ada dulu sebelum
// create/list, dan mengembalikan ErrNotFound yang jelas, bukan mengandalkan
// foreign key violation. Pola ini sama persis dengan job.CostService.
type MachineService struct {
	repo         MachineRepository
	customerRepo Repository
}

func NewMachineService(repo MachineRepository, customerRepo Repository) *MachineService {
	return &MachineService{repo: repo, customerRepo: customerRepo}
}

type CreateMachineInput struct {
	CustomerID   uuid.UUID
	MachineName  string
	MachineType  *string
	SerialNumber *string
	Notes        *string
}

func (s *MachineService) Create(ctx context.Context, in CreateMachineInput) (Machine, error) {
	if _, err := s.customerRepo.GetByID(ctx, in.CustomerID); err != nil {
		return Machine{}, err
	}

	m := Machine{
		CustomerID:   in.CustomerID,
		MachineName:  in.MachineName,
		MachineType:  in.MachineType,
		SerialNumber: in.SerialNumber,
		Notes:        in.Notes,
	}
	if err := m.Validate(); err != nil {
		return Machine{}, err
	}

	created, err := s.repo.Create(ctx, m)
	if err != nil {
		return Machine{}, fmt.Errorf("create machine: %w", err)
	}
	return created, nil
}

func (s *MachineService) ListByCustomer(ctx context.Context, customerID uuid.UUID) ([]Machine, error) {
	if _, err := s.customerRepo.GetByID(ctx, customerID); err != nil {
		return nil, err
	}

	machines, err := s.repo.ListByCustomer(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("list machines: %w", err)
	}
	return machines, nil
}
