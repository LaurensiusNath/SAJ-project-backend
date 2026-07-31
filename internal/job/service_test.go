package job

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nathan/cnc-pm-backend/internal/customer"
	"github.com/nathan/cnc-pm-backend/internal/notification"
	"github.com/nathan/cnc-pm-backend/internal/user"
)

// noopCustomerRepo/noopUserRepo/noopNotifier: job.Service's tests di file
// ini menguji business logic job (status transition, assign, dst), BUKAN
// perilaku notifikasi (itu tanggung jawab internal/notification/service_test.go
// sendiri) - jadi fake di sini sengaja selalu "not found"/no-op, supaya
// notifyAssignment/notifyJobCompleted diam-diam skip tanpa memengaruhi
// assertion test manapun.
type noopCustomerRepo struct{}

func (noopCustomerRepo) Create(context.Context, customer.Customer) (customer.Customer, error) {
	return customer.Customer{}, customer.ErrNotFound
}
func (noopCustomerRepo) GetByID(context.Context, uuid.UUID) (customer.Customer, error) {
	return customer.Customer{}, customer.ErrNotFound
}
func (noopCustomerRepo) List(context.Context, customer.ListFilter, int32, int32) ([]customer.Customer, error) {
	return nil, nil
}
func (noopCustomerRepo) Count(context.Context, customer.ListFilter) (int64, error) { return 0, nil }
func (noopCustomerRepo) Update(context.Context, customer.Customer) (customer.Customer, error) {
	return customer.Customer{}, customer.ErrNotFound
}
func (noopCustomerRepo) Delete(context.Context, uuid.UUID) error { return nil }

type noopUserRepo struct{}

func (noopUserRepo) Create(context.Context, user.User) (user.User, error) {
	return user.User{}, user.ErrNotFound
}
func (noopUserRepo) GetByEmail(context.Context, string) (user.User, error) {
	return user.User{}, user.ErrNotFound
}
func (noopUserRepo) GetByID(context.Context, uuid.UUID) (user.User, error) {
	return user.User{}, user.ErrNotFound
}

type noopNotifier struct{}

func (noopNotifier) SendEmail(context.Context, notification.EmailInput) error { return nil }

// noopCostRepo: job.Service.GetDetail butuh CostRepository - test di file
// ini tidak menguji perilaku job_costs (itu tanggung jawab
// cost_service_test.go sendiri), jadi fake ini selalu mengembalikan slice
// kosong.
type noopCostRepo struct{}

func (noopCostRepo) Create(context.Context, JobCost) (JobCost, error) { return JobCost{}, nil }
func (noopCostRepo) ListByJob(context.Context, uuid.UUID) ([]JobCost, error) {
	return nil, nil
}
func (noopCostRepo) Totals(context.Context, uuid.UUID) (CostTotals, error) {
	return CostTotals{}, nil
}
func (noopCostRepo) Delete(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (noopCostRepo) InvoiceTotals(context.Context, uuid.UUID) (InvoiceCostTotals, error) {
	return InvoiceCostTotals{}, nil
}

func newTestService(repo Repository) *Service {
	return NewService(repo, noopCustomerRepo{}, noopUserRepo{}, noopCostRepo{}, noopNotifier{})
}

// stubCustomerRepo/stubUserRepo/recordingNotifier - dipakai KHUSUS test
// yang membuktikan notifikasi benar-benar terpicu dengan data yang benar
// (bukan cuma "tidak panic" seperti noop di atas).
type stubCustomerRepo struct {
	noopCustomerRepo
	customer customer.Customer
}

func (s stubCustomerRepo) GetByID(context.Context, uuid.UUID) (customer.Customer, error) {
	return s.customer, nil
}

type stubUserRepo struct {
	noopUserRepo
	user user.User
}

func (s stubUserRepo) GetByID(context.Context, uuid.UUID) (user.User, error) {
	return s.user, nil
}

type recordingNotifier struct {
	sent []notification.EmailInput
}

func (r *recordingNotifier) SendEmail(_ context.Context, in notification.EmailInput) error {
	r.sent = append(r.sent, in)
	return nil
}

// fakeRepository - in-memory, tulisan tangan, sama pola dengan
// internal/customer/service_test.go.
type fakeRepository struct {
	jobs          map[uuid.UUID]Job
	statusHistory map[uuid.UUID][]JobStatusHistory
	tick          int
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		jobs:          make(map[uuid.UUID]Job),
		statusHistory: make(map[uuid.UUID][]JobStatusHistory),
	}
}

// nextTimestamp menghindari time.Now() dipanggil dua kali super cepat
// (mis. Create lalu langsung AssignTechnician di test yang sama) dan
// menghasilkan nilai yang identik karena resolusi jam OS - itu akan
// membuat test optimistic locking salah lolos (staleUpdatedAt kebetulan
// masih "match" walau sudah ada perubahan).
func (f *fakeRepository) nextTimestamp() time.Time {
	f.tick++
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(f.tick) * time.Second)
}

func (f *fakeRepository) Create(_ context.Context, j Job) (Job, error) {
	j.ID = uuid.New()
	j.JobCode = "JOB-TEST-0001"
	j.CreatedAt = f.nextTimestamp()
	j.UpdatedAt = j.CreatedAt
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

func (f *fakeRepository) ListNeedingReminderCheck(_ context.Context) ([]Job, error) {
	var result []Job
	for _, j := range f.jobs {
		if j.Status != StatusCompleted && j.Status != StatusCancelled && j.ScheduledDate != nil {
			result = append(result, j)
		}
	}
	return result, nil
}

func (f *fakeRepository) UpdateStatus(_ context.Context, id uuid.UUID, status JobStatus, completedDate *time.Time, changedBy uuid.UUID, notes *string) (Job, error) {
	j, ok := f.jobs[id]
	if !ok {
		return Job{}, ErrNotFound
	}
	j.Status = status
	j.CompletedDate = completedDate
	f.jobs[id] = j
	f.statusHistory[id] = append(f.statusHistory[id], JobStatusHistory{
		ID:        uuid.New(),
		JobID:     id,
		Status:    status,
		ChangedBy: changedBy,
		ChangedAt: f.nextTimestamp(),
		Notes:     notes,
	})
	return j, nil
}

func (f *fakeRepository) ListStatusHistory(_ context.Context, jobID uuid.UUID) ([]JobStatusHistory, error) {
	return f.statusHistory[jobID], nil
}

func (f *fakeRepository) AssignTechnician(_ context.Context, id, technicianID uuid.UUID, expectedUpdatedAt time.Time) (Job, error) {
	j, ok := f.jobs[id]
	if !ok {
		return Job{}, ErrNotFound
	}
	if !j.UpdatedAt.Equal(expectedUpdatedAt) {
		return Job{}, ErrConflict
	}
	j.TechnicianID = &technicianID
	j.UpdatedAt = f.nextTimestamp()
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
			svc := newTestService(newFakeRepository())

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
	svc := newTestService(newFakeRepository())

	_, err := svc.GetByID(context.Background(), uuid.New())

	require.ErrorIs(t, err, ErrNotFound)
}

func TestService_List_FiltersByStatusAndCounts(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)
	ctx := context.Background()
	customerID := uuid.New()
	for range 2 {
		_, err := svc.Create(ctx, CreateInput{CustomerID: customerID, Title: "Job requested"})
		require.NoError(t, err)
	}
	completed, err := svc.Create(ctx, CreateInput{CustomerID: customerID, Title: "Job completed"})
	require.NoError(t, err)
	_, err = svc.UpdateStatus(ctx, completed.ID, StatusCompleted, uuid.New(), nil)
	require.NoError(t, err)

	requested := StatusRequested
	result, err := svc.List(ctx, ListParams{Status: &requested})

	require.NoError(t, err)
	assert.Equal(t, int64(2), result.Total)
	assert.Len(t, result.Jobs, 2)
}

func TestService_UpdateStatus_SetsAndClearsCompletedDate(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)
	ctx := context.Background()
	created, err := svc.Create(ctx, CreateInput{CustomerID: uuid.New(), Title: "Servis"})
	require.NoError(t, err)

	completed, err := svc.UpdateStatus(ctx, created.ID, StatusCompleted, uuid.New(), nil)
	require.NoError(t, err)
	require.NotNil(t, completed.CompletedDate, "completed_date should be set once status becomes completed")

	reopened, err := svc.UpdateStatus(ctx, created.ID, StatusInProgress, uuid.New(), nil)
	require.NoError(t, err)
	assert.Nil(t, reopened.CompletedDate, "completed_date should be cleared if job is reopened")
}

func TestService_UpdateStatus_RejectsInvalidStatus(t *testing.T) {
	svc := newTestService(newFakeRepository())

	_, err := svc.UpdateStatus(context.Background(), uuid.New(), JobStatus("bukan-status-valid"), uuid.New(), nil)

	require.ErrorIs(t, err, ErrInvalidStatus)
}

func TestService_UpdateStatus_NotFound(t *testing.T) {
	svc := newTestService(newFakeRepository())

	_, err := svc.UpdateStatus(context.Background(), uuid.New(), StatusScheduled, uuid.New(), nil)

	require.ErrorIs(t, err, ErrNotFound)
}

func TestService_AssignTechnician(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)
	created, err := svc.Create(context.Background(), CreateInput{CustomerID: uuid.New(), Title: "Servis"})
	require.NoError(t, err)
	technicianID := uuid.New()

	updated, err := svc.AssignTechnician(context.Background(), created.ID, technicianID, created.UpdatedAt)

	require.NoError(t, err)
	require.NotNil(t, updated.TechnicianID)
	assert.Equal(t, technicianID, *updated.TechnicianID)
}

func TestService_AssignTechnician_NotFound(t *testing.T) {
	svc := newTestService(newFakeRepository())

	_, err := svc.AssignTechnician(context.Background(), uuid.New(), uuid.New(), time.Now())

	require.ErrorIs(t, err, ErrNotFound)
}

// TestService_AssignTechnician_ConflictOnStaleUpdatedAt membuktikan
// optimistic locking: admin B mengirim expected_updated_at yang sudah usang
// (karena admin A sudah assign duluan) harus ditolak dengan ErrConflict,
// bukan diam-diam menimpa pilihan admin A.
func TestService_AssignTechnician_ConflictOnStaleUpdatedAt(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)
	created, err := svc.Create(context.Background(), CreateInput{CustomerID: uuid.New(), Title: "Servis"})
	require.NoError(t, err)
	staleUpdatedAt := created.UpdatedAt

	technicianA := uuid.New()
	_, err = svc.AssignTechnician(context.Background(), created.ID, technicianA, staleUpdatedAt)
	require.NoError(t, err, "admin A assigns first using the updated_at both admins originally read")

	technicianB := uuid.New()
	_, err = svc.AssignTechnician(context.Background(), created.ID, technicianB, staleUpdatedAt)
	require.ErrorIs(t, err, ErrConflict, "admin B still holds the pre-A updated_at, so this must be rejected instead of silently overwriting")
}

// TestService_AssignTechnician_NotifiesCustomerAndTechnician membuktikan
// AssignTechnician benar-benar memicu DUA email (customer + teknisi),
// bukan cuma tidak error - lihat notifyAssignment di service.go.
func TestService_AssignTechnician_NotifiesCustomerAndTechnician(t *testing.T) {
	repo := newFakeRepository()
	custEmail := "customer@example.com"
	techEmail := "teknisi@example.com"
	notifier := &recordingNotifier{}
	svc := NewService(
		repo,
		stubCustomerRepo{customer: customer.Customer{Email: &custEmail}},
		stubUserRepo{user: user.User{Email: techEmail}},
		noopCostRepo{},
		notifier,
	)
	created, err := svc.Create(context.Background(), CreateInput{CustomerID: uuid.New(), Title: "Servis"})
	require.NoError(t, err)

	_, err = svc.AssignTechnician(context.Background(), created.ID, uuid.New(), created.UpdatedAt)

	require.NoError(t, err)
	require.Len(t, notifier.sent, 2, "should notify both the customer and the assigned technician")
	recipients := []string{notifier.sent[0].To, notifier.sent[1].To}
	assert.Contains(t, recipients, custEmail)
	assert.Contains(t, recipients, techEmail)
}

// TestService_UpdateStatus_Completed_NotifiesCustomer membuktikan job yang
// jadi "completed" memicu satu email ke customer - dan job yang berubah ke
// status lain TIDAK memicu apa-apa (bukan status yang relevan dinotifikasi).
func TestService_UpdateStatus_Completed_NotifiesCustomer(t *testing.T) {
	repo := newFakeRepository()
	custEmail := "customer@example.com"
	notifier := &recordingNotifier{}
	svc := NewService(repo, stubCustomerRepo{customer: customer.Customer{Email: &custEmail}}, noopUserRepo{}, noopCostRepo{}, notifier)
	created, err := svc.Create(context.Background(), CreateInput{CustomerID: uuid.New(), Title: "Servis"})
	require.NoError(t, err)

	_, err = svc.UpdateStatus(context.Background(), created.ID, StatusInProgress, uuid.New(), nil)
	require.NoError(t, err)
	assert.Empty(t, notifier.sent, "in_progress should not trigger any notification")

	_, err = svc.UpdateStatus(context.Background(), created.ID, StatusCompleted, uuid.New(), nil)

	require.NoError(t, err)
	require.Len(t, notifier.sent, 1)
	assert.Equal(t, custEmail, notifier.sent[0].To)
}

// TestService_UpdateStatus_RecordsHistory membuktikan requirement dari
// docs/api-contract.md: "Setiap perubahan wajib insert row baru ke
// job_status_history" (status, changed_by dari JWT claim, notes).
func TestService_UpdateStatus_RecordsHistory(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)
	created, err := svc.Create(context.Background(), CreateInput{CustomerID: uuid.New(), Title: "Servis"})
	require.NoError(t, err)
	changedBy := uuid.New()
	notes := "Teknisi sudah di lokasi"

	_, err = svc.UpdateStatus(context.Background(), created.ID, StatusInProgress, changedBy, &notes)

	require.NoError(t, err)
	history, err := repo.ListStatusHistory(context.Background(), created.ID)
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, StatusInProgress, history[0].Status)
	assert.Equal(t, changedBy, history[0].ChangedBy)
	require.NotNil(t, history[0].Notes)
	assert.Equal(t, notes, *history[0].Notes)
}

// TestService_UpdateStatus_RecordsHistory_MultipleTransitions membuktikan
// riwayat terakumulasi (bukan menimpa) tiap kali status berubah lagi, urut
// kronologis - GET /jobs/{id} butuh urutan ini apa adanya (lihat GetDetail).
func TestService_UpdateStatus_RecordsHistory_MultipleTransitions(t *testing.T) {
	repo := newFakeRepository()
	svc := newTestService(repo)
	created, err := svc.Create(context.Background(), CreateInput{CustomerID: uuid.New(), Title: "Servis"})
	require.NoError(t, err)

	_, err = svc.UpdateStatus(context.Background(), created.ID, StatusScheduled, uuid.New(), nil)
	require.NoError(t, err)
	_, err = svc.UpdateStatus(context.Background(), created.ID, StatusInProgress, uuid.New(), nil)
	require.NoError(t, err)
	_, err = svc.UpdateStatus(context.Background(), created.ID, StatusCompleted, uuid.New(), nil)
	require.NoError(t, err)

	history, err := repo.ListStatusHistory(context.Background(), created.ID)

	require.NoError(t, err)
	require.Len(t, history, 3)
	assert.Equal(t, StatusScheduled, history[0].Status)
	assert.Equal(t, StatusInProgress, history[1].Status)
	assert.Equal(t, StatusCompleted, history[2].Status)
}

// TestService_GetDetail_ComposesJobHistoryAndCosts membuktikan GET /jobs/{id}
// mengembalikan Job + status_history + costs dalam satu response (lihat
// docs/api-contract.md: "wajib nested, ini requirement, bukan opsional").
func TestService_GetDetail_ComposesJobHistoryAndCosts(t *testing.T) {
	jobRepo := newFakeRepository()
	costRepo := newFakeCostRepository()
	svc := NewService(jobRepo, noopCustomerRepo{}, noopUserRepo{}, costRepo, noopNotifier{})
	created, err := svc.Create(context.Background(), CreateInput{CustomerID: uuid.New(), Title: "Servis"})
	require.NoError(t, err)

	changedBy := uuid.New()
	_, err = svc.UpdateStatus(context.Background(), created.ID, StatusInProgress, changedBy, nil)
	require.NoError(t, err)

	_, err = costRepo.Create(context.Background(), JobCost{
		JobID: created.ID, CostType: CostTypeLabor, Description: "Jasa",
		Quantity: decimal.NewFromInt(1), SellingPrice: decimal.NewFromInt(1),
	})
	require.NoError(t, err)

	detail, err := svc.GetDetail(context.Background(), created.ID)

	require.NoError(t, err)
	assert.Equal(t, created.ID, detail.ID, "Detail embeds Job fields directly")
	require.Len(t, detail.StatusHistory, 1)
	assert.Equal(t, StatusInProgress, detail.StatusHistory[0].Status)
	require.Len(t, detail.Costs, 1)
}

func TestService_GetDetail_NotFound(t *testing.T) {
	svc := newTestService(newFakeRepository())

	_, err := svc.GetDetail(context.Background(), uuid.New())

	require.ErrorIs(t, err, ErrNotFound)
}
