//go:build integration

package invoice_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
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

// setupBadanUsahaInvoice seeds one customer (badan_usaha - required so
// ComputeAmounts actually produces a non-zero pph23_estimated_amount,
// otherwise the scenarios below couldn't exercise the
// bukti-potong-flips-effectivePaid path at all), one completed job with a
// single labor cost, and returns the resulting draft invoice plus the
// invoice.Repository to act on it. Shared by both
// TestUpdatePaymentBuktiPotong_* tests below.
func setupBadanUsahaInvoice(t *testing.T, pool *pgxpool.Pool, queries *sqlcgen.Queries, laborAmount decimal.Decimal) invoice.Invoice {
	t.Helper()
	ctx := context.Background()

	custRepo := customer.NewRepository(queries)
	cust, err := custRepo.Create(ctx, customer.Customer{
		Name: "Bukti Potong Test Co", CustomerType: customer.CustomerTypeBadanUsaha,
	})
	require.NoError(t, err)

	userRepo := user.NewRepository(queries)
	tester, err := userRepo.Create(ctx, user.User{
		Name: "Bukti Potong Integration Tester", Email: "integration-bukti-potong-" + uuid.NewString() + "@example.com",
		PasswordHash: "x", Role: user.RoleOwner,
	})
	require.NoError(t, err)

	jobRepo := job.NewRepository(pool, queries)
	j, err := jobRepo.Create(ctx, job.Job{CustomerID: cust.ID, Title: "Bukti potong test job"})
	require.NoError(t, err)

	costRepo := job.NewCostRepository(queries)
	_, err = costRepo.Create(ctx, job.JobCost{
		JobID: j.ID, CostType: job.CostTypeLabor, Description: "Jasa servis",
		Quantity: decimal.NewFromInt(1), SellingPrice: laborAmount,
	})
	require.NoError(t, err)

	_, err = jobRepo.UpdateStatus(ctx, j.ID, job.StatusCompleted, nil, tester.ID, nil)
	require.NoError(t, err)

	invoiceRepo := invoice.NewRepository(pool, queries, noopSender{})
	inv, err := invoiceRepo.CreateFromJob(ctx, invoice.CreateFromJobInput{JobID: j.ID})
	require.NoError(t, err)
	return inv
}

// TestUpdatePaymentBuktiPotong_FillsInLateBuktiPotong_MakesInvoicePaid is
// the mandatory "isi bukti potong bikin invoice jadi paid" scenario: money
// alone (totalPaid) isn't enough to cross inv.Total, but totalPaid +
// pph23_estimated_amount is - exactly the situation a bukti potong document
// arriving AFTER the payment was recorded is meant to resolve (see
// repository.go UpdatePaymentBuktiPotong doc comment for the full
// analysis). Recording the payment WITHOUT a ref first (instead of passing
// it directly to RecordPayment) is the point of this test - it proves the
// PATCH endpoint, not just RecordPayment's own existing bukti-potong path.
func TestUpdatePaymentBuktiPotong_FillsInLateBuktiPotong_MakesInvoicePaid(t *testing.T) {
	pool := testhelper.NewPostgresPool(t)
	ctx := context.Background()
	queries := sqlcgen.New(pool)

	inv := setupBadanUsahaInvoice(t, pool, queries, decimal.NewFromInt(1000000))
	// subtotal 1,000,000, tax 11% -> tax_amount 110,000, total 1,110,000.
	// dpp_pph23 = 1,000,000 (labor saja) -> pph23_estimated_amount (badan
	// usaha, rate 2%) = 20,000.
	require.True(t, inv.Total.Equal(decimal.NewFromInt(1110000)), "sanity check, got %s", inv.Total)
	require.True(t, inv.PPh23EstimatedAmount.Equal(decimal.NewFromInt(20000)), "sanity check, got %s", inv.PPh23EstimatedAmount)

	invoiceRepo := invoice.NewRepository(pool, queries, noopSender{})

	// totalPaid (1,090,000) sendirian TIDAK cukup (< 1,110,000) - invoice
	// harus tetap draft/belum lunas sampai bukti potong diisi.
	payment, err := invoiceRepo.RecordPayment(ctx, invoice.RecordPaymentInput{
		InvoiceID: inv.ID, Amount: decimal.NewFromInt(1090000), PaymentMethod: invoice.PaymentMethodTransfer,
	})
	require.NoError(t, err)

	afterPayment, err := invoiceRepo.GetByID(ctx, inv.ID)
	require.NoError(t, err)
	require.NotEqual(t, invoice.StatusPaid, afterPayment.Status,
		"sanity check: money alone (1,090,000) must NOT be enough to mark this paid (total is 1,110,000)")

	ref := "BP-LATE-001"
	updated, err := invoiceRepo.UpdatePaymentBuktiPotong(ctx, inv.ID, payment.ID, ref)
	require.NoError(t, err)
	require.NotNil(t, updated.BuktiPotongPPh23Ref)
	assert.Equal(t, ref, *updated.BuktiPotongPPh23Ref)

	final, err := invoiceRepo.GetByID(ctx, inv.ID)
	require.NoError(t, err)
	assert.Equal(t, invoice.StatusPaid, final.Status,
		"1,090,000 (money) + 20,000 (pph23_estimated_amount, now counted because this payment has a bukti potong ref) = 1,110,000 = total -> must be paid")
}

// TestUpdatePaymentBuktiPotong_AlreadyPaidByOtherPayment_StatusUnchanged is
// the mandatory "isi bukti potong yang TIDAK mengubah status karena sudah
// paid dari payment lain" scenario: two payments together already sum to
// the full total (no pph23 credit needed at all), so the invoice is ALREADY
// 'paid' before the bukti potong PATCH ever runs. Filling in a bukti potong
// on one of them afterward must be a pure record-correction with zero
// effect on status - proving the endpoint doesn't blindly re-run the paid
// transition without it actually being warranted.
func TestUpdatePaymentBuktiPotong_AlreadyPaidByOtherPayment_StatusUnchanged(t *testing.T) {
	pool := testhelper.NewPostgresPool(t)
	ctx := context.Background()
	queries := sqlcgen.New(pool)

	inv := setupBadanUsahaInvoice(t, pool, queries, decimal.NewFromInt(1000000))
	require.True(t, inv.Total.Equal(decimal.NewFromInt(1110000)), "sanity check, got %s", inv.Total)

	invoiceRepo := invoice.NewRepository(pool, queries, noopSender{})

	paymentA, err := invoiceRepo.RecordPayment(ctx, invoice.RecordPaymentInput{
		InvoiceID: inv.ID, Amount: decimal.NewFromInt(600000), PaymentMethod: invoice.PaymentMethodTransfer,
	})
	require.NoError(t, err)
	// paymentB alone pushes SUM(payments.amount) to 1,110,000 = total - the
	// invoice becomes 'paid' from money alone here, BEFORE any bukti potong
	// is ever involved.
	_, err = invoiceRepo.RecordPayment(ctx, invoice.RecordPaymentInput{
		InvoiceID: inv.ID, Amount: decimal.NewFromInt(510000), PaymentMethod: invoice.PaymentMethodTransfer,
	})
	require.NoError(t, err)

	beforePatch, err := invoiceRepo.GetByID(ctx, inv.ID)
	require.NoError(t, err)
	require.Equal(t, invoice.StatusPaid, beforePatch.Status, "sanity check: two payments alone must already total the full amount")

	updated, err := invoiceRepo.UpdatePaymentBuktiPotong(ctx, inv.ID, paymentA.ID, "BP-AFTER-PAID-001")
	require.NoError(t, err)
	require.NotNil(t, updated.BuktiPotongPPh23Ref)
	assert.Equal(t, "BP-AFTER-PAID-001", *updated.BuktiPotongPPh23Ref, "the ref itself must still be recorded correctly")

	final, err := invoiceRepo.GetByID(ctx, inv.ID)
	require.NoError(t, err)
	assert.Equal(t, invoice.StatusPaid, final.Status, "status was ALREADY paid from payments alone - filling bukti potong afterward changes nothing about it")
}

// TestUpdatePaymentBuktiPotong_ConcurrentWithNewPayment_NoLostUpdate proves
// the exact race the mandatory investigation identified: RecordPayment and
// UpdatePaymentBuktiPotong BOTH read+write invoice.status based on
// SUM(payments)+pph23_estimated_amount, so a new payment arriving at
// nearly the same time as a bukti potong being filled in on an EXISTING
// payment is just as much a lost-update risk as two concurrent
// RecordPayment calls (see TestRecordPayment_ConcurrentPayments_NoLostUpdate
// above) - just triggered by two DIFFERENT operations instead of the same
// one twice. GetInvoiceForUpdate's row lock must serialize these two
// different code paths against each other, not just against themselves.
func TestUpdatePaymentBuktiPotong_ConcurrentWithNewPayment_NoLostUpdate(t *testing.T) {
	pool := testhelper.NewPostgresPool(t)
	ctx := context.Background()
	queries := sqlcgen.New(pool)

	inv := setupBadanUsahaInvoice(t, pool, queries, decimal.NewFromInt(1000000))
	require.True(t, inv.Total.Equal(decimal.NewFromInt(1110000)), "sanity check, got %s", inv.Total)
	require.True(t, inv.PPh23EstimatedAmount.Equal(decimal.NewFromInt(20000)), "sanity check, got %s", inv.PPh23EstimatedAmount)

	invoiceRepo := invoice.NewRepository(pool, queries, noopSender{})

	// First payment recorded sequentially (not part of the race) - WITHOUT
	// a bukti potong ref yet, this is exactly the "bukti potong arrives
	// late" situation.
	paymentA, err := invoiceRepo.RecordPayment(ctx, invoice.RecordPaymentInput{
		InvoiceID: inv.ID, Amount: decimal.NewFromInt(700000), PaymentMethod: invoice.PaymentMethodTransfer,
	})
	require.NoError(t, err)

	// Without the row lock: goroutine A could read totalPaid=700,000 (not
	// yet seeing B's +390,000) and compute effectivePaid=700,000+20,000=
	// 720,000 (< 1,110,000); goroutine B could read totalPaid=1,090,000 (not
	// yet seeing A's bukti potong) with hasBuktiPotong=false, computing
	// effectivePaid=1,090,000 (< 1,110,000) - NEITHER would mark it paid,
	// even though together (1,090,000 money + 20,000 pph23 credit =
	// 1,110,000) the invoice is fully settled. This is the lost update
	// GetInvoiceForUpdate's FOR UPDATE lock must prevent by serializing the
	// two transactions.
	var wg sync.WaitGroup
	var recordErr, patchErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, recordErr = invoiceRepo.RecordPayment(ctx, invoice.RecordPaymentInput{
			InvoiceID: inv.ID, Amount: decimal.NewFromInt(390000), PaymentMethod: invoice.PaymentMethodCash,
		})
	}()
	go func() {
		defer wg.Done()
		_, patchErr = invoiceRepo.UpdatePaymentBuktiPotong(ctx, inv.ID, paymentA.ID, "BP-RACE-001")
	}()
	wg.Wait()
	require.NoError(t, recordErr)
	require.NoError(t, patchErr)

	final, err := invoiceRepo.GetByID(ctx, inv.ID)
	require.NoError(t, err)
	assert.Equal(t, invoice.StatusPaid, final.Status,
		"700,000+390,000 (money) + 20,000 (pph23 credit, now counted) = 1,110,000 = total - must be paid regardless of which of the two concurrent operations committed first")
}
