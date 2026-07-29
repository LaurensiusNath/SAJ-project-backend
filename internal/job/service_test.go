package job

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRepository - in-memory, tulisan tangan, sama pola dengan
// internal/customer/service_test.go.
type fakeRepository struct {
	jobs map[uuid.UUID]Job
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{jobs: make(map[uuid.UUID]Job)}
}

func (f *fakeRepository) Create(_ context.Context, j Job) (Job, error) {
	j.ID = uuid.New()
	j.JobCode = "JOB-TEST-0001"
	j.CreatedAt = time.Now()
	j.UpdatedAt = time.Now()
	f.jobs[j.ID] = j
	return j, nil
}

func (f *fakeRepository) GetByID(_ context.Context, id uuid.UUID) (Job, error) {
	j, ok := f.jobs[id]
	if !ok {
		return Job{}, ErrNotFound
	}
	return j, nil
}

func (f *fakeRepository) matches(j Job, filter ListFilter) bool {
	if filter.Status != nil && j.Status != *filter.Status {
		return false
	}
	if filter.CustomerID != nil && j.CustomerID != *filter.CustomerID {
		return false
	}
	return true
}

func (f *fakeRepository) filtered(filter ListFilter) []Job {
	result := make([]Job, 0, len(f.jobs))
	for _, j := range f.jobs {
		if f.matches(j, filter) {
			result = append(result, j)
		}
	}
	sort.SliceStable(result, func(i, k int) bool { return result[i].CreatedAt.After(result[k].CreatedAt) })
	return result
}

func (f *fakeRepository) List(_ context.Context, filter ListFilter, limit, offset int32) ([]Job, error) {
	all := f.filtered(filter)
	start := min(int(offset), len(all))
	end := min(start+int(limit), len(all))
	return all[start:end], nil
}

func (f *fakeRepository) Count(_ context.Context, filter ListFilter) (int64, error) {
	return int64(len(f.filtered(filter))), nil
}

func (f *fakeRepository) UpdateStatus(_ context.Context, id uuid.UUID, status JobStatus, completedDate *time.Time) (Job, error) {
	j, ok := f.jobs[id]
	if !ok {
		return Job{}, ErrNotFound
	}
	j.Status = status
	j.CompletedDate = completedDate
	f.jobs[id] = j
	return j, nil
}

func (f *fakeRepository) AssignTechnician(_ context.Context, id, technicianID uuid.UUID) (Job, error) {
	j, ok := f.jobs[id]
	if !ok {
		return Job{}, ErrNotFound
	}
	j.TechnicianID = &technicianID
	f.jobs[id] = j
	return j, nil
}

func TestService_Create(t *testing.T) {
	customerID := uuid.New()

	testCases := []struct {
		name    string
		input   CreateInput
		wantErr error
	}{
		{
			name:    "valid job",
			input:   CreateInput{CustomerID: customerID, Title: "Servis rutin CNC"},
			wantErr: nil,
		},
		{
			name:    "empty title is rejected",
			input:   CreateInput{CustomerID: customerID, Title: ""},
			wantErr: ErrInvalidTitle,
		},
		{
			name:    "missing customer_id is rejected",
			input:   CreateInput{Title: "Servis rutin CNC"},
			wantErr: ErrInvalidCustomer,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(newFakeRepository())

			got, err := svc.Create(context.Background(), tc.input)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.NotEqual(t, uuid.Nil, got.ID)
			assert.Equal(t, StatusRequested, got.Status, "new job should always start as requested")
		})
	}
}

func TestService_GetByID_NotFound(t *testing.T) {
	svc := NewService(newFakeRepository())

	_, err := svc.GetByID(context.Background(), uuid.New())

	require.ErrorIs(t, err, ErrNotFound)
}

func TestService_List_FiltersByStatusAndCounts(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ctx := context.Background()
	customerID := uuid.New()
	for range 2 {
		_, err := svc.Create(ctx, CreateInput{CustomerID: customerID, Title: "Job requested"})
		require.NoError(t, err)
	}
	completed, err := svc.Create(ctx, CreateInput{CustomerID: customerID, Title: "Job completed"})
	require.NoError(t, err)
	_, err = svc.UpdateStatus(ctx, completed.ID, StatusCompleted)
	require.NoError(t, err)

	requested := StatusRequested
	result, err := svc.List(ctx, ListParams{Status: &requested})

	require.NoError(t, err)
	assert.Equal(t, int64(2), result.Total)
	assert.Len(t, result.Jobs, 2)
}

func TestService_UpdateStatus_SetsAndClearsCompletedDate(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ctx := context.Background()
	created, err := svc.Create(ctx, CreateInput{CustomerID: uuid.New(), Title: "Servis"})
	require.NoError(t, err)

	completed, err := svc.UpdateStatus(ctx, created.ID, StatusCompleted)
	require.NoError(t, err)
	require.NotNil(t, completed.CompletedDate, "completed_date should be set once status becomes completed")

	reopened, err := svc.UpdateStatus(ctx, created.ID, StatusInProgress)
	require.NoError(t, err)
	assert.Nil(t, reopened.CompletedDate, "completed_date should be cleared if job is reopened")
}

func TestService_UpdateStatus_RejectsInvalidStatus(t *testing.T) {
	svc := NewService(newFakeRepository())

	_, err := svc.UpdateStatus(context.Background(), uuid.New(), JobStatus("bukan-status-valid"))

	require.ErrorIs(t, err, ErrInvalidStatus)
}

func TestService_UpdateStatus_NotFound(t *testing.T) {
	svc := NewService(newFakeRepository())

	_, err := svc.UpdateStatus(context.Background(), uuid.New(), StatusScheduled)

	require.ErrorIs(t, err, ErrNotFound)
}

func TestService_AssignTechnician(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	created, err := svc.Create(context.Background(), CreateInput{CustomerID: uuid.New(), Title: "Servis"})
	require.NoError(t, err)
	technicianID := uuid.New()

	updated, err := svc.AssignTechnician(context.Background(), created.ID, technicianID)

	require.NoError(t, err)
	require.NotNil(t, updated.TechnicianID)
	assert.Equal(t, technicianID, *updated.TechnicianID)
}

func TestService_AssignTechnician_NotFound(t *testing.T) {
	svc := NewService(newFakeRepository())

	_, err := svc.AssignTechnician(context.Background(), uuid.New(), uuid.New())

	require.ErrorIs(t, err, ErrNotFound)
}
