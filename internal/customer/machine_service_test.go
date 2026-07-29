package customer

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeMachineRepository struct {
	machines map[uuid.UUID]Machine
}

func newFakeMachineRepository() *fakeMachineRepository {
	return &fakeMachineRepository{machines: make(map[uuid.UUID]Machine)}
}

func (f *fakeMachineRepository) Create(_ context.Context, m Machine) (Machine, error) {
	m.ID = uuid.New()
	m.CreatedAt = time.Now()
	m.UpdatedAt = time.Now()
	f.machines[m.ID] = m
	return m, nil
}

func (f *fakeMachineRepository) ListByCustomer(_ context.Context, customerID uuid.UUID) ([]Machine, error) {
	var result []Machine
	for _, m := range f.machines {
		if m.CustomerID == customerID {
			result = append(result, m)
		}
	}
	return result, nil
}

// seedCustomer menaruh satu Customer langsung ke fakeRepository supaya
// MachineService.customerRepo.GetByID punya sesuatu untuk ditemukan - sama
// pola dengan seedJob di job/cost_service_test.go.
func seedCustomer(t *testing.T, repo *fakeRepository) uuid.UUID {
	t.Helper()
	created, err := repo.Create(context.Background(), Customer{
		Name:         "Customer untuk test machine",
		CustomerType: CustomerTypePerorangan,
	})
	require.NoError(t, err)
	return created.ID
}

func TestMachineService_Create(t *testing.T) {
	customerRepo := newFakeRepository()
	customerID := seedCustomer(t, customerRepo)

	testCases := []struct {
		name    string
		input   CreateMachineInput
		wantErr error
	}{
		{
			name:  "valid machine",
			input: CreateMachineInput{CustomerID: customerID, MachineName: "CNC Milling XYZ-100"},
		},
		{
			name:    "empty machine_name is rejected",
			input:   CreateMachineInput{CustomerID: customerID, MachineName: ""},
			wantErr: ErrInvalidMachineName,
		},
		{
			name:    "nonexistent customer returns not found",
			input:   CreateMachineInput{CustomerID: uuid.New(), MachineName: "CNC Milling XYZ-100"},
			wantErr: ErrNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewMachineService(newFakeMachineRepository(), customerRepo)

			got, err := svc.Create(context.Background(), tc.input)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.input.MachineName, got.MachineName)
		})
	}
}

func TestMachineService_ListByCustomer(t *testing.T) {
	customerRepo := newFakeRepository()
	customerID := seedCustomer(t, customerRepo)
	machineRepo := newFakeMachineRepository()
	svc := NewMachineService(machineRepo, customerRepo)
	ctx := context.Background()

	_, err := svc.Create(ctx, CreateMachineInput{CustomerID: customerID, MachineName: "Mesin A"})
	require.NoError(t, err)
	_, err = svc.Create(ctx, CreateMachineInput{CustomerID: customerID, MachineName: "Mesin B"})
	require.NoError(t, err)

	machines, err := svc.ListByCustomer(ctx, customerID)

	require.NoError(t, err)
	assert.Len(t, machines, 2)
}

func TestMachineService_ListByCustomer_CustomerNotFound(t *testing.T) {
	svc := NewMachineService(newFakeMachineRepository(), newFakeRepository())

	_, err := svc.ListByCustomer(context.Background(), uuid.New())

	require.ErrorIs(t, err, ErrNotFound)
}
