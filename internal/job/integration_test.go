//go:build integration

package job_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nathan/cnc-pm-backend/internal/customer"
	"github.com/nathan/cnc-pm-backend/internal/job"
	"github.com/nathan/cnc-pm-backend/internal/repository/sqlcgen"
	"github.com/nathan/cnc-pm-backend/internal/testhelper"
	"github.com/nathan/cnc-pm-backend/internal/user"
)

// TestAssignTechnician_ConcurrentAssign_OnlyOneSucceeds is the automated,
// CI-enforced replacement for the manual proof done earlier in this
// project that optimistic locking on PATCH /jobs/{id}/assign correctly
// rejects a stale concurrent assignment instead of silently letting one
// admin's choice overwrite another's.
//
// Runs against a real Postgres (testcontainers-go) because the guarantee
// being tested lives entirely in the database's WHERE-clause row match
// ("UPDATE ... WHERE id = $1 AND updated_at = $2") - a fake in-memory
// Repository would need to reimplement that exact compare-and-swap
// semantic itself, which would just be testing the fake, not the real
// SQL behavior.
func TestAssignTechnician_ConcurrentAssign_OnlyOneSucceeds(t *testing.T) {
	pool := testhelper.NewPostgresPool(t)
	ctx := context.Background()
	queries := sqlcgen.New(pool)

	custRepo := customer.NewRepository(queries)
	cust, err := custRepo.Create(ctx, customer.Customer{
		Name: "Integration Test Co", CustomerType: customer.CustomerTypePerorangan,
	})
	require.NoError(t, err)

	userRepo := user.NewRepository(queries)
	techA, err := userRepo.Create(ctx, user.User{
		Name: "Teknisi A", Email: "integration-teknisi-a@example.com", PasswordHash: "x", Role: user.RoleTeknisi,
	})
	require.NoError(t, err)
	techB, err := userRepo.Create(ctx, user.User{
		Name: "Teknisi B", Email: "integration-teknisi-b@example.com", PasswordHash: "x", Role: user.RoleTeknisi,
	})
	require.NoError(t, err)

	jobRepo := job.NewRepository(pool, queries)
	j, err := jobRepo.Create(ctx, job.Job{CustomerID: cust.ID, Title: "Integration test job"})
	require.NoError(t, err)

	// Kedua goroutine memakai j.UpdatedAt yang SAMA (dibaca sebelum
	// keduanya jalan) - mensimulasikan dua admin yang sama-sama membaca
	// job ini sebelum salah satu assign duluan.
	technicians := []uuid.UUID{techA.ID, techB.ID}
	const n = 2
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := jobRepo.AssignTechnician(ctx, j.ID, technicians[i], j.UpdatedAt)
			errs[i] = err
		}(i)
	}
	wg.Wait()

	var successCount, conflictCount int
	for i, err := range errs {
		switch {
		case err == nil:
			successCount++
		case errors.Is(err, job.ErrConflict):
			conflictCount++
		default:
			t.Fatalf("goroutine %d: unexpected error: %v", i, err)
		}
	}
	assert.Equal(t, 1, successCount, "exactly one of the two concurrent assigns should succeed")
	assert.Equal(t, 1, conflictCount, "the other must be rejected with ErrConflict (409), not silently overwrite the winner")

	final, err := jobRepo.GetByID(ctx, j.ID)
	require.NoError(t, err)
	require.NotNil(t, final.TechnicianID)
	assert.Contains(t, technicians, *final.TechnicianID, "the job must end up assigned to whichever technician won, not left unassigned or corrupted")
}
