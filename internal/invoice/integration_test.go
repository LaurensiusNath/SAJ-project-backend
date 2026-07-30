//go:build integration

package invoice_test

import (
	"context"
	"sync"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nathan/cnc-pm-backend/internal/customer"
	"github.com/nathan/cnc-pm-backend/internal/invoice"
	"github.com/nathan/cnc-pm-backend/internal/job"
	"github.com/nathan/cnc-pm-backend/internal/notification"
	"github.com/nathan/cnc-pm-backend/internal/repository/sqlcgen"
	"github.com/nathan/cnc-pm-backend/internal/testhelper"
	"github.com/nathan/cnc-pm-backend/internal/user"
)

type noopSender struct{}

func (noopSender) SendEmail(context.Context, notification.EmailInput) error { return nil }

// TestRecordPayment_ConcurrentPayments_NoLostUpdate is the automated,
// CI-enforced replacement for the manual proof done earlier in this
// project (a throwaway program, run once against WSL2 Postgres, then
// deleted - see project history) that SELECT ... FOR UPDATE on the
// invoice row prevents a lost update when multiple payments arrive at
// nearly the same time.
//
// This MUST run against a real Postgres (testcontainers-go here, not a
// fake Repository): the race being proven only exists at the database
// transaction/locking level. A fake, in-memory Repository has no
// equivalent of "two transactions reading the same row before either
// commits" - it would never be able to reproduce or disprove this bug.
func TestRecordPayment_ConcurrentPayments_NoLostUpdate(t *testing.T) {
	pool := testhelper.NewPostgresPool(t)
	ctx := context.Background()
	queries := sqlcgen.New(pool)

	custRepo := customer.NewRepository(queries)
	cust, err := custRepo.Create(ctx, customer.Customer{
		Name: "Integration Test Co", CustomerType: customer.CustomerTypePerorangan,
	})
	require.NoError(t, err)

	userRepo := user.NewRepository(queries)
	tester, err := userRepo.Create(ctx, user.User{
		Name: "Integration Tester", Email: "integration-payment@example.com",
		PasswordHash: "x", Role: user.RoleOwner,
	})
	require.NoError(t, err)

	jobRepo := job.NewRepository(pool, queries)
	j, err := jobRepo.Create(ctx, job.Job{CustomerID: cust.ID, Title: "Integration test job"})
	require.NoError(t, err)

	costRepo := job.NewCostRepository(queries)
	_, err = costRepo.Create(ctx, job.JobCost{
		JobID: j.ID, CostType: job.CostTypeLabor, Description: "Jasa servis",
		Quantity: decimal.NewFromInt(1), SellingPrice: decimal.NewFromInt(400000),
	})
	require.NoError(t, err)

	_, err = jobRepo.UpdateStatus(ctx, j.ID, job.StatusCompleted, nil, tester.ID, nil)
	require.NoError(t, err)

	invoiceRepo := invoice.NewRepository(pool, queries, noopSender{})
	inv, err := invoiceRepo.CreateFromJob(ctx, invoice.CreateFromJobInput{JobID: j.ID})
	require.NoError(t, err)
	// subtotal 400000, tax 11% (default company_settings) -> total 444000
	require.True(t, inv.Total.Equal(decimal.NewFromInt(444000)), "sanity check on the amount used below, got %s", inv.Total)

	_, err = invoiceRepo.UpdateStatus(ctx, inv.ID, invoice.StatusSent)
	require.NoError(t, err)

	// 4 pembayaran x 111000 = 444000 = total invoice persis - kalau
	// SELECT ... FOR UPDATE tidak ada, race lost-update membuat status
	// tetap "sent" walau uangnya sudah lengkap (persis bug yang pernah
	// dibuktikan manual sebelumnya).
	const n = 4
	amount := decimal.NewFromInt(111000)
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := invoiceRepo.RecordPayment(ctx, invoice.RecordPaymentInput{
				InvoiceID: inv.ID, Amount: amount, PaymentMethod: invoice.PaymentMethodTransfer,
			})
			errs[i] = err
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		require.NoError(t, err, "payment goroutine %d failed", i)
	}

	final, err := invoiceRepo.GetByID(ctx, inv.ID)
	require.NoError(t, err)
	assert.Equal(t, invoice.StatusPaid, final.Status,
		"combined payments equal total exactly - invoice must be 'paid', not stuck at 'sent' due to a lost update")

	payments, err := invoiceRepo.ListPayments(ctx, inv.ID)
	require.NoError(t, err)
	require.Len(t, payments, n)
	sum := decimal.Zero
	for _, p := range payments {
		sum = sum.Add(p.Amount)
	}
	assert.True(t, sum.Equal(final.Total), "sum of recorded payments should equal invoice total, got %s vs %s", sum, final.Total)
}
