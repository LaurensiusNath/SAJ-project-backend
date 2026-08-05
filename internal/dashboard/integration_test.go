//go:build integration

package dashboard_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nathan/cnc-pm-backend/internal/customer"
	"github.com/nathan/cnc-pm-backend/internal/dashboard"
	"github.com/nathan/cnc-pm-backend/internal/dateonly"
	"github.com/nathan/cnc-pm-backend/internal/job"
	"github.com/nathan/cnc-pm-backend/internal/pgconv"
	"github.com/nathan/cnc-pm-backend/internal/repository/sqlcgen"
	"github.com/nathan/cnc-pm-backend/internal/testhelper"
	"github.com/nathan/cnc-pm-backend/internal/user"
)

// insertInvoice/insertPayment menembus langsung ke tabel lewat SQL mentah,
// BUKAN lewat invoice.Repository (CreateFromJob/RecordPayment) - sengaja,
// karena test ini menguji sisi BACA (dashboard mengagregasi dengan benar),
// bukan sisi tulis invoice/payment (itu sudah dibuktikan terpisah di
// internal/invoice/integration_test.go). Insert langsung memberi kontrol
// penuh atas status dan created_at, termasuk membuat data yang SENGAJA di
// luar period yang akan di-query - sesuatu yang tidak mungkin dilakukan
// lewat CreateFromJob (created_at selalu now()).
func insertInvoice(t *testing.T, pool *pgxpool.Pool, jobID uuid.UUID, invoiceNumber string, total decimal.Decimal, status string, createdAt time.Time) uuid.UUID {
	t.Helper()
	var id pgtype.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO invoices (
			invoice_number, job_id, subtotal, tax_percentage, tax_amount, total,
			dpp_pph23, pph23_rate, pph23_estimated_amount, expected_receivable,
			status, created_at, updated_at
		) VALUES ($1, $2, $3, 0, 0, $3, 0, 0, 0, $3, $4, $5, $5)
		RETURNING id`,
		invoiceNumber, pgconv.ToUUID(jobID), pgconv.ToNumeric(total), status, pgconv.ToTimestamptz(createdAt),
	).Scan(&id)
	require.NoError(t, err)
	return pgconv.FromUUID(id)
}

func insertPayment(t *testing.T, pool *pgxpool.Pool, invoiceID uuid.UUID, amount decimal.Decimal, createdAt time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO payments (invoice_id, amount, payment_method, created_at)
		VALUES ($1, $2, 'transfer', $3)`,
		pgconv.ToUUID(invoiceID), pgconv.ToNumeric(amount), pgconv.ToTimestamptz(createdAt),
	)
	require.NoError(t, err)
}

// TestGetSummary_AggregatesAcrossCustomersJobsInvoicesPayments seeds a
// scenario spanning 2 customers, 12 jobs (7 "real" jobs for the
// scheduling/status assertions + 5 dedicated invoice-carrier jobs), 5
// invoices, and 3 payments, deliberately straddling the period boundary
// (period = [today-3, today], inclusive) so every number below is only
// correct if the period filter, the +1 day exclusive upper bound, and the
// all-time (not period-scoped) outstanding_total are ALL implemented
// correctly - this is exactly the class of silent off-by-one / wrong-scope
// bug the report was asked to guard against.
func TestGetSummary_AggregatesAcrossCustomersJobsInvoicesPayments(t *testing.T) {
	pool := testhelper.NewPostgresPool(t)
	ctx := context.Background()
	queries := sqlcgen.New(pool)

	custRepo := customer.NewRepository(queries)
	custA, err := custRepo.Create(ctx, customer.Customer{Name: "Bengkel A", CustomerType: customer.CustomerTypePerorangan})
	require.NoError(t, err)
	custB, err := custRepo.Create(ctx, customer.Customer{Name: "Pabrik B", CustomerType: customer.CustomerTypeBadanUsaha})
	require.NoError(t, err)

	userRepo := user.NewRepository(queries)
	tester, err := userRepo.Create(ctx, user.User{
		Name: "Dashboard Integration Tester", Email: "integration-dashboard@example.com",
		PasswordHash: "x", Role: user.RoleOwner,
	})
	require.NoError(t, err)

	jobRepo := job.NewRepository(pool, queries)
	today := time.Now().UTC().Truncate(24 * time.Hour)
	dateP := func(d int) *dateonly.Date {
		date := dateonly.FromTime(today.AddDate(0, 0, d))
		return &date
	}

	// --- jobs untuk assertion by_status / upcoming / overdue ---
	j1, err := jobRepo.Create(ctx, job.Job{CustomerID: custA.ID, Title: "J1 upcoming near", ScheduledDate: dateP(2)})
	require.NoError(t, err)
	j2, err := jobRepo.Create(ctx, job.Job{CustomerID: custA.ID, Title: "J2 upcoming edge +7", ScheduledDate: dateP(7)})
	require.NoError(t, err)
	_, err = jobRepo.UpdateStatus(ctx, j2.ID, job.StatusScheduled, nil, tester.ID, nil)
	require.NoError(t, err)
	j3, err := jobRepo.Create(ctx, job.Job{CustomerID: custB.ID, Title: "J3 overdue", ScheduledDate: dateP(-3)})
	require.NoError(t, err)
	_, err = jobRepo.UpdateStatus(ctx, j3.ID, job.StatusInProgress, nil, tester.ID, nil)
	require.NoError(t, err)
	j4, err := jobRepo.Create(ctx, job.Job{CustomerID: custB.ID, Title: "J4 completed, past, must NOT be overdue", ScheduledDate: dateP(-5)})
	require.NoError(t, err)
	completedAt := dateonly.FromTime(today)
	_, err = jobRepo.UpdateStatus(ctx, j4.ID, job.StatusCompleted, &completedAt, tester.ID, nil)
	require.NoError(t, err)
	j5, err := jobRepo.Create(ctx, job.Job{CustomerID: custA.ID, Title: "J5 cancelled, future, must NOT be upcoming", ScheduledDate: dateP(1)})
	require.NoError(t, err)
	_, err = jobRepo.UpdateStatus(ctx, j5.ID, job.StatusCancelled, nil, tester.ID, nil)
	require.NoError(t, err)
	_, err = jobRepo.Create(ctx, job.Job{CustomerID: custA.ID, Title: "J6 too far (+10), not upcoming", ScheduledDate: dateP(10)})
	require.NoError(t, err)
	_, err = jobRepo.Create(ctx, job.Job{CustomerID: custB.ID, Title: "J7 no scheduled_date at all"})
	require.NoError(t, err)

	// --- 5 job "pembawa invoice" terpisah - statusnya tidak relevan untuk
	// assertion di atas, sengaja completed+tanpa scheduled_date supaya tidak
	// ikut kehitung di upcoming/overdue dan tidak mengacaukan hitungan manual.
	invoiceJobIDs := make([]uuid.UUID, 5)
	for i := 0; i < 5; i++ {
		ij, err := jobRepo.Create(ctx, job.Job{CustomerID: custA.ID, Title: "invoice carrier job"})
		require.NoError(t, err)
		_, err = jobRepo.UpdateStatus(ctx, ij.ID, job.StatusCompleted, &completedAt, tester.ID, nil)
		require.NoError(t, err)
		invoiceJobIDs[i] = ij.ID
	}

	// --- invoices, straddling period = [today-3, today] ---
	inv1 := insertInvoice(t, pool, invoiceJobIDs[0], "INV-DASH-1", decimal.NewFromInt(100), "sent", today.AddDate(0, 0, -1)) // in period
	insertInvoice(t, pool, invoiceJobIDs[1], "INV-DASH-2", decimal.NewFromInt(200), "paid", today.AddDate(0, 0, -2))         // in period
	insertInvoice(t, pool, invoiceJobIDs[2], "INV-DASH-3", decimal.NewFromInt(150), "overdue", today.AddDate(0, 0, -10))     // OUTSIDE period (before period_from)
	insertInvoice(t, pool, invoiceJobIDs[3], "INV-DASH-4", decimal.NewFromInt(80), "cancelled", today.AddDate(0, 0, -1))     // in period
	insertInvoice(t, pool, invoiceJobIDs[4], "INV-DASH-5", decimal.NewFromInt(50), "draft", today)                           // in period, exactly on period_to (last day)

	// --- payments ---
	insertPayment(t, pool, inv1, decimal.NewFromInt(40), today.AddDate(0, 0, -1))  // in period
	insertPayment(t, pool, inv1, decimal.NewFromInt(10), today.AddDate(0, 0, -20)) // OUTSIDE period, but must still count toward outstanding_total (all-time)
	// inv2 (paid) - actual payment amount doesn't matter for these assertions, left unpaid at the payments-table level on purpose (status was set directly).
	// inv3 (overdue) - sengaja TIDAK dibayar sama sekali.

	repo := dashboard.NewRepository(pool, queries)
	summary, err := repo.GetSummary(ctx, dashboard.PeriodInput{
		From:        today.AddDate(0, 0, -3),
		To:          today,
		ToExclusive: today.AddDate(0, 0, 1),
	})
	require.NoError(t, err)

	// --- financial ---
	assert.True(t, summary.Financial.InvoicedTotal.Equal(decimal.NewFromInt(300)),
		"invoiced_total: only sent (100) + paid (200) count - draft (50) and cancelled (80) are excluded even though in-period, overdue inv3 (150) excluded because it's outside the period - got %s", summary.Financial.InvoicedTotal)
	// Property that must ALWAYS hold: invoiced_total is defined as
	// sent+paid+overdue, so it must equal the sum of exactly those three
	// by_status buckets - this is the regression guard asked for after the
	// draft/cancelled inclusion bug, see also the dedicated
	// TestGetSummary_InvoicedTotal_EqualsSentPlusPaidPlusOverdue below.
	byStatus := summary.Financial.ByStatus
	assert.True(t, summary.Financial.InvoicedTotal.Equal(byStatus.Sent.Total.Add(byStatus.Paid.Total).Add(byStatus.Overdue.Total)),
		"invoiced_total (%s) must equal sent.total+paid.total+overdue.total (%s+%s+%s)",
		summary.Financial.InvoicedTotal, byStatus.Sent.Total, byStatus.Paid.Total, byStatus.Overdue.Total)
	assert.True(t, summary.Financial.ReceivedTotal.Equal(decimal.NewFromInt(40)),
		"received_total: only the 40 payment is in-period, the 10 payment is outside period - got %s", summary.Financial.ReceivedTotal)
	assert.True(t, summary.Financial.OutstandingTotal.Equal(decimal.NewFromInt(200)),
		"outstanding_total: (100+150 invoiced for sent/overdue) - (40+10 ever paid against them, ALL-TIME not period-scoped) = 200 - got %s", summary.Financial.OutstandingTotal)

	assert.Equal(t, int64(1), summary.Financial.ByStatus.Draft.Count)
	assert.True(t, summary.Financial.ByStatus.Draft.Total.Equal(decimal.NewFromInt(50)))
	assert.Equal(t, int64(1), summary.Financial.ByStatus.Sent.Count)
	assert.True(t, summary.Financial.ByStatus.Sent.Total.Equal(decimal.NewFromInt(100)))
	assert.Equal(t, int64(1), summary.Financial.ByStatus.Paid.Count)
	assert.True(t, summary.Financial.ByStatus.Paid.Total.Equal(decimal.NewFromInt(200)))
	assert.Equal(t, int64(0), summary.Financial.ByStatus.Overdue.Count,
		"inv3 IS overdue-status but its created_at is outside the period, so the PERIOD-scoped by_status breakdown must not count it")
	assert.True(t, summary.Financial.ByStatus.Overdue.Total.IsZero())
	assert.Equal(t, int64(1), summary.Financial.ByStatus.Cancelled.Count)
	assert.True(t, summary.Financial.ByStatus.Cancelled.Total.Equal(decimal.NewFromInt(80)))

	// --- jobs ---
	assert.Equal(t, int64(3), summary.Jobs.ByStatus.Requested, "J1, J6, J7")
	assert.Equal(t, int64(1), summary.Jobs.ByStatus.Scheduled, "J2")
	assert.Equal(t, int64(1), summary.Jobs.ByStatus.InProgress, "J3")
	assert.Equal(t, int64(6), summary.Jobs.ByStatus.Completed, "J4 + 5 invoice-carrier jobs")
	assert.Equal(t, int64(1), summary.Jobs.ByStatus.Cancelled, "J5")

	require.Len(t, summary.Jobs.Upcoming7Days, 2)
	assert.Equal(t, j1.ID, summary.Jobs.Upcoming7Days[0].ID, "J1 (+2 days) must come before J2 (+7 days)")
	assert.Equal(t, j2.ID, summary.Jobs.Upcoming7Days[1].ID, "J2 at exactly +7 days must be INCLUDED (inclusive upper bound)")
	assert.Equal(t, "Bengkel A", summary.Jobs.Upcoming7Days[0].CustomerName)

	require.Len(t, summary.Jobs.OverdueScheduled, 1)
	assert.Equal(t, j3.ID, summary.Jobs.OverdueScheduled[0].ID)
	assert.Equal(t, "Pabrik B", summary.Jobs.OverdueScheduled[0].CustomerName)
}

// TestGetSummary_InvoicedTotal_EqualsSentPlusPaidPlusOverdue is a dedicated
// regression guard for a real bug caught in review: invoiced_total used to
// sum ALL invoices in the period regardless of status, which silently
// counted draft (not yet issued) and cancelled invoices as "invoiced". This
// test seeds exactly one invoice per status in the SAME period and asserts
// the property that must always hold: invoiced_total is defined as
// sent+paid+overdue, nothing else - if this logic drifts again (e.g.
// someone "simplifies" the WHERE clause back to no status filter), this
// test fails immediately instead of relying on someone noticing the
// discrepancy against by_status by eye.
func TestGetSummary_InvoicedTotal_EqualsSentPlusPaidPlusOverdue(t *testing.T) {
	pool := testhelper.NewPostgresPool(t)
	ctx := context.Background()
	queries := sqlcgen.New(pool)

	custRepo := customer.NewRepository(queries)
	cust, err := custRepo.Create(ctx, customer.Customer{Name: "Invoiced Total Test Co", CustomerType: customer.CustomerTypePerorangan})
	require.NoError(t, err)
	jobRepo := job.NewRepository(pool, queries)

	today := time.Now().UTC().Truncate(24 * time.Hour)
	statuses := []struct {
		status string
		total  int64
	}{
		{"draft", 10}, {"cancelled", 20}, {"sent", 30}, {"paid", 40}, {"overdue", 50},
	}
	for _, s := range statuses {
		j, err := jobRepo.Create(ctx, job.Job{CustomerID: cust.ID, Title: "carrier for " + s.status})
		require.NoError(t, err)
		insertInvoice(t, pool, j.ID, "INV-PROP-"+s.status, decimal.NewFromInt(s.total), s.status, today)
	}

	repo := dashboard.NewRepository(pool, queries)
	summary, err := repo.GetSummary(ctx, dashboard.PeriodInput{
		From: today.AddDate(0, 0, -1), To: today, ToExclusive: today.AddDate(0, 0, 1),
	})
	require.NoError(t, err)

	byStatus := summary.Financial.ByStatus
	expected := byStatus.Sent.Total.Add(byStatus.Paid.Total).Add(byStatus.Overdue.Total)
	assert.True(t, summary.Financial.InvoicedTotal.Equal(expected),
		"invoiced_total (%s) must equal sent+paid+overdue (%s) - got draft=%s cancelled=%s",
		summary.Financial.InvoicedTotal, expected, byStatus.Draft.Total, byStatus.Cancelled.Total)
	// Pinned concretely too, not just the property - 30+40+50, draft(10) and cancelled(20) excluded.
	assert.True(t, summary.Financial.InvoicedTotal.Equal(decimal.NewFromInt(120)),
		"got %s, want 120 (30 sent + 40 paid + 50 overdue, draft/cancelled excluded)", summary.Financial.InvoicedTotal)
}

// TestGetSummary_RepeatableRead_DoesNotSeeConcurrentCommit proves the exact
// Postgres guarantee the report leans on: once this transaction's first
// query establishes its snapshot, a write that COMMITS from a different
// connection while our transaction is still open must stay invisible to us
// until we start a new transaction. This is what makes GetSummary's several
// separate SELECTs internally consistent with each other - without it, two
// numbers in the same response could reflect two different instants.
//
// Driven directly against pgx (not through dashboard.Repository.GetSummary,
// which opens and commits its own transaction in one call with no seam to
// interleave a concurrent write into) - this test proves the primitive
// GetSummary relies on, the same way the primitive itself would be tested.
func TestGetSummary_RepeatableRead_DoesNotSeeConcurrentCommit(t *testing.T) {
	pool := testhelper.NewPostgresPool(t)
	ctx := context.Background()
	queries := sqlcgen.New(pool)

	custRepo := customer.NewRepository(queries)
	cust, err := custRepo.Create(ctx, customer.Customer{Name: "Snapshot Test Co", CustomerType: customer.CustomerTypePerorangan})
	require.NoError(t, err)
	jobRepo := job.NewRepository(pool, queries)
	jobA, err := jobRepo.Create(ctx, job.Job{CustomerID: cust.ID, Title: "carrier A"})
	require.NoError(t, err)
	jobB, err := jobRepo.Create(ctx, job.Job{CustomerID: cust.ID, Title: "carrier B"})
	require.NoError(t, err)

	// status "sent" (not "draft") - InvoicedTotal only counts
	// sent/paid/overdue, this test needs a status that actually counts so
	// the sanity check below is meaningful.
	now := time.Now().UTC()
	insertInvoice(t, pool, jobA.ID, "INV-SNAP-1", decimal.NewFromInt(100), "sent", now)

	period := sqlcgen.InvoicedTotalParams{
		PeriodFrom:        pgconv.ToTimestamptz(now.AddDate(0, 0, -1)),
		PeriodToExclusive: pgconv.ToTimestamptz(now.AddDate(0, 0, 1)),
	}

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	txQueries := queries.WithTx(tx)

	firstRead, err := txQueries.InvoicedTotal(ctx, period)
	require.NoError(t, err)
	require.True(t, pgconv.FromNumeric(firstRead).Equal(decimal.NewFromInt(100)), "sanity check before the concurrent write")

	// Commits from a SEPARATE connection (the base pool, autocommit) while
	// tx above is still open - this is the "concurrent write" that a
	// Read-Committed transaction WOULD see on its next statement, but a
	// Repeatable Read transaction must not.
	insertInvoice(t, pool, jobB.ID, "INV-SNAP-2", decimal.NewFromInt(500), "sent", now)

	secondRead, err := txQueries.InvoicedTotal(ctx, period)
	require.NoError(t, err)
	assert.True(t, pgconv.FromNumeric(secondRead).Equal(decimal.NewFromInt(100)),
		"REPEATABLE READ transaction must still see the snapshot from its first query (100), not the concurrently committed 600 - got %s", pgconv.FromNumeric(secondRead))

	require.NoError(t, tx.Commit(ctx))

	// A fresh statement (no explicit transaction = each gets its own
	// Read Committed snapshot) now sees both invoices - proves the second
	// invoice really was committed and visible, just not to the still-open
	// Repeatable Read transaction above.
	afterCommit, err := queries.InvoicedTotal(ctx, period)
	require.NoError(t, err)
	assert.True(t, pgconv.FromNumeric(afterCommit).Equal(decimal.NewFromInt(600)),
		"after the snapshot transaction ends, a new read must see both invoices (100+500=600) - got %s", pgconv.FromNumeric(afterCommit))
}
