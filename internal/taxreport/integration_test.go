//go:build integration

package taxreport_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nathan/cnc-pm-backend/internal/customer"
	"github.com/nathan/cnc-pm-backend/internal/job"
	"github.com/nathan/cnc-pm-backend/internal/pgconv"
	"github.com/nathan/cnc-pm-backend/internal/repository/sqlcgen"
	"github.com/nathan/cnc-pm-backend/internal/taxreport"
	"github.com/nathan/cnc-pm-backend/internal/testhelper"
)

// insertInvoice menembus langsung ke tabel invoices lewat SQL mentah (bukan
// lewat invoice.Repository.CreateFromJob) - sama alasan dengan
// dashboard/integration_test.go: test ini menguji sisi BACA taxreport, dan
// insert langsung memberi kontrol penuh atas status, created_at, DAN
// pph23_estimated_amount (CreateFromJob menghitungnya sendiri dari job_costs,
// terlalu tidak langsung untuk menyusun skenario alokasi proporsional yang
// presisi di bawah).
func insertInvoice(t *testing.T, pool *pgxpool.Pool, jobID uuid.UUID, invoiceNumber string, subtotal, taxAmount, pph23Estimated decimal.Decimal, status string, createdAt time.Time) uuid.UUID {
	t.Helper()
	total := subtotal.Add(taxAmount)
	var id pgtype.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO invoices (
			invoice_number, job_id, subtotal, tax_percentage, tax_amount, total,
			dpp_pph23, pph23_rate, pph23_estimated_amount, expected_receivable,
			status, created_at, updated_at
		) VALUES ($1, $2, $3, 11, $4, $5, 0, 2, $6, $5, $7, $8, $8)
		RETURNING id`,
		invoiceNumber, pgconv.ToUUID(jobID), pgconv.ToNumeric(subtotal), pgconv.ToNumeric(taxAmount),
		pgconv.ToNumeric(total), pgconv.ToNumeric(pph23Estimated), status, pgconv.ToTimestamptz(createdAt),
	).Scan(&id)
	require.NoError(t, err)
	return pgconv.FromUUID(id)
}

func insertPayment(t *testing.T, pool *pgxpool.Pool, invoiceID uuid.UUID, amount decimal.Decimal, createdAt time.Time, buktiPotongRef *string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO payments (invoice_id, amount, payment_method, bukti_potong_pph23_ref, created_at)
		VALUES ($1, $2, 'transfer', $3, $4)`,
		pgconv.ToUUID(invoiceID), pgconv.ToNumeric(amount), pgconv.ToText(buktiPotongRef), pgconv.ToTimestamptz(createdAt),
	)
	require.NoError(t, err)
}

func ref(s string) *string { return &s }

// TestGetSummary_MixedData_FiltersAndAllocatesCorrectly adalah satu skenario
// besar yang sengaja mencampur SEMUA kasus penting sekaligus (bukan
// dipecah jadi banyak test kecil dengan setup terpisah - GetSummary
// mengagregasi lintas invoice+payment jadi paling representatif diuji lewat
// satu dataset yang beririsan), membuktikan:
//
//  1. ppn.invoices HANYA berisi status sent/paid/overdue (draft & cancelled
//     dikecualikan) DAN hanya yang created_at-nya di dalam period.
//  2. total_ppn_keluaran = SUM(tax_amount) persis dari invoice yang lolos
//     filter di atas, tidak lebih tidak kurang.
//  3. pph23.payments TIDAK memfilter berdasar status invoice-nya - payment
//     terhadap invoice DRAFT tetap muncul (asimetris dengan ppn.invoices,
//     lihat db/queries/taxreport.sql).
//  4. pph23.payments memfilter berdasar payments.created_at sendiri (BUKAN
//     invoices.created_at) - payment di luar period dikecualikan walau
//     invoice induknya di dalam period.
//  5. pph23_share_estimasi dialokasikan PROPORSIONAL ke tiap payment suatu
//     invoice (bukan ditumpuk di payment pertama, bukan diulang penuh di
//     semua payment) - SUM(share) satu invoice harus persis sama dengan
//     pph23_estimated_amount invoice itu.
//  6. customer_type diekspos apa adanya (BUKAN dipakai untuk filter) -
//     payment dari customer perorangan tetap muncul, dengan share 0 karena
//     pph23_estimated_amount invoice-nya memang 0 (bukan karena difilter).
func TestGetSummary_MixedData_FiltersAndAllocatesCorrectly(t *testing.T) {
	pool := testhelper.NewPostgresPool(t)
	ctx := context.Background()
	queries := sqlcgen.New(pool)

	custRepo := customer.NewRepository(queries)
	custBadan, err := custRepo.Create(ctx, customer.Customer{Name: "PT Mitra Utama", CustomerType: customer.CustomerTypeBadanUsaha})
	require.NoError(t, err)
	custPerorangan, err := custRepo.Create(ctx, customer.Customer{Name: "Budi Santoso", CustomerType: customer.CustomerTypePerorangan})
	require.NoError(t, err)

	jobRepo := job.NewRepository(pool, queries)
	newCarrierJob := func(custID uuid.UUID) uuid.UUID {
		j, err := jobRepo.Create(ctx, job.Job{CustomerID: custID, Title: "invoice carrier job"})
		require.NoError(t, err)
		return j.ID
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	inPeriod := today.AddDate(0, 0, -1)
	outOfPeriod := today.AddDate(0, 0, -20)

	// --- invoices ---
	// 1. draft - HARUS TIDAK muncul di ppn.invoices, tax_amount 999 dipilih
	//    besar supaya kalau bug filter-nya hilang, total_ppn_keluaran akan
	//    jelas salah (bukan kebetulan cocok).
	insertInvoice(t, pool, newCarrierJob(custBadan.ID), "INV-TAX-DRAFT",
		decimal.NewFromInt(9000), decimal.NewFromInt(999), decimal.Zero, "draft", inPeriod)
	// 2. cancelled - HARUS TIDAK muncul.
	insertInvoice(t, pool, newCarrierJob(custBadan.ID), "INV-TAX-CANCELLED",
		decimal.NewFromInt(8000), decimal.NewFromInt(888), decimal.Zero, "cancelled", inPeriod)
	// 3. sent, badan usaha, 2 payment (60/40 split) - membuktikan alokasi proporsional.
	inv3 := insertInvoice(t, pool, newCarrierJob(custBadan.ID), "INV-TAX-SENT",
		decimal.NewFromInt(1000), decimal.NewFromInt(110), decimal.NewFromInt(20), "sent", inPeriod)
	insertPayment(t, pool, inv3, decimal.NewFromInt(60), inPeriod, ref("BP-001"))
	insertPayment(t, pool, inv3, decimal.NewFromInt(40), inPeriod, nil)
	// 4. paid, badan usaha, 1 payment penuh - kasus paling sederhana (share == pph23_estimated_amount).
	inv4 := insertInvoice(t, pool, newCarrierJob(custBadan.ID), "INV-TAX-PAID",
		decimal.NewFromInt(500), decimal.NewFromInt(55), decimal.NewFromInt(10), "paid", inPeriod)
	insertPayment(t, pool, inv4, decimal.NewFromInt(100), inPeriod, ref("BP-002"))
	// 5. overdue, tanpa payment sama sekali - harus tetap muncul di ppn.invoices,
	//    tapi tidak menyumbang apapun ke pph23.payments.
	insertInvoice(t, pool, newCarrierJob(custBadan.ID), "INV-TAX-OVERDUE",
		decimal.NewFromInt(300), decimal.NewFromInt(33), decimal.Zero, "overdue", inPeriod)
	// 6. sent, TAPI created_at DI LUAR period - HARUS TIDAK muncul di
	//    ppn.invoices (period filter di sisi invoice).
	insertInvoice(t, pool, newCarrierJob(custBadan.ID), "INV-TAX-OUTOFPERIOD",
		decimal.NewFromInt(4545), decimal.NewFromInt(500), decimal.Zero, "sent", outOfPeriod)
	// 7. sent, customer PERORANGAN - pph23_estimated_amount 0 (skip PPh23 untuk
	//    perorangan sudah benar di invoice.ComputeAmounts, lihat temuan
	//    investigasi) - payment-nya harus tetap muncul (customer_type
	//    diekspos, BUKAN difilter), dengan share 0.
	inv7 := insertInvoice(t, pool, newCarrierJob(custPerorangan.ID), "INV-TAX-PERORANGAN",
		decimal.NewFromInt(200), decimal.NewFromInt(22), decimal.Zero, "sent", inPeriod)
	insertPayment(t, pool, inv7, decimal.NewFromInt(200), inPeriod, nil)
	// 8. DRAFT, tapi PUNYA payment - membuktikan pph23.payments TIDAK
	//    memfilter berdasar status invoice (asimetris dengan ppn.invoices,
	//    yang justru mengecualikan invoice draft ini).
	inv8 := insertInvoice(t, pool, newCarrierJob(custBadan.ID), "INV-TAX-DRAFT-WITH-PAYMENT",
		decimal.NewFromInt(700), decimal.NewFromInt(77), decimal.NewFromInt(15), "draft", inPeriod)
	insertPayment(t, pool, inv8, decimal.NewFromInt(100), inPeriod, nil)
	// 9. sent, DI DALAM period (jadi ppn.invoices tetap menghitungnya) -
	//    tapi payment-nya DI LUAR period - membuktikan filter payments pakai
	//    payments.created_at sendiri, bukan invoices.created_at.
	inv9 := insertInvoice(t, pool, newCarrierJob(custBadan.ID), "INV-TAX-PAYMENT-OUTOFPERIOD",
		decimal.NewFromInt(455), decimal.NewFromInt(50), decimal.NewFromInt(5), "sent", inPeriod)
	insertPayment(t, pool, inv9, decimal.NewFromInt(100), outOfPeriod, nil)

	repo := taxreport.NewRepository(queries)
	summary, err := repo.GetSummary(ctx, taxreport.PeriodInput{
		From:        today.AddDate(0, 0, -3),
		To:          today,
		ToExclusive: today.AddDate(0, 0, 1),
	})
	require.NoError(t, err)

	// --- ppn.invoices ---
	// sent(110) + paid(55) + overdue(33) + perorangan-sent(22) + inv9-sent(50) = 270
	// draft(999), cancelled(888), out-of-period(500), draft-with-payment(77) dikecualikan.
	assert.True(t, summary.PPN.TotalPPNKeluaran.Equal(decimal.NewFromInt(270)),
		"total_ppn_keluaran: got %s, want 270 (110+55+33+22+50, draft/cancelled/out-of-period/draft-with-payment excluded)", summary.PPN.TotalPPNKeluaran)
	assert.Len(t, summary.PPN.Invoices, 5, "hanya 5 invoice sent/paid/overdue yang created_at-nya di dalam period")
	invoiceNumbers := make(map[string]bool)
	for _, inv := range summary.PPN.Invoices {
		invoiceNumbers[inv.InvoiceNumber] = true
	}
	assert.True(t, invoiceNumbers["INV-TAX-SENT"])
	assert.True(t, invoiceNumbers["INV-TAX-PAID"])
	assert.True(t, invoiceNumbers["INV-TAX-OVERDUE"])
	assert.True(t, invoiceNumbers["INV-TAX-PERORANGAN"])
	assert.True(t, invoiceNumbers["INV-TAX-PAYMENT-OUTOFPERIOD"])
	assert.False(t, invoiceNumbers["INV-TAX-DRAFT"], "draft harus dikecualikan")
	assert.False(t, invoiceNumbers["INV-TAX-CANCELLED"], "cancelled harus dikecualikan")
	assert.False(t, invoiceNumbers["INV-TAX-OUTOFPERIOD"], "created_at di luar period harus dikecualikan")
	assert.False(t, invoiceNumbers["INV-TAX-DRAFT-WITH-PAYMENT"], "draft harus dikecualikan walau punya payment")

	// --- pph23.payments ---
	// pay3a(12) + pay3b(8) + pay4(10) + pay7(0) + pay8(15) = 45
	// pay9 (di luar period) dikecualikan.
	assert.True(t, summary.PPh23.TotalEstimasi.Equal(decimal.NewFromInt(45)),
		"total_estimasi: got %s, want 45 (12+8+10+0+15, payment di luar period dikecualikan)", summary.PPh23.TotalEstimasi)
	require.Len(t, summary.PPh23.Payments, 5, "5 payment di dalam period: 2 dari inv3, 1 dari inv4, 1 dari inv7, 1 dari inv8 - pay9 di luar period dikecualikan")

	// byRef ambil payment PERTAMA per invoice_number - cukup untuk INV-TAX-PAID/
	// INV-TAX-PERORANGAN/INV-TAX-DRAFT-WITH-PAYMENT yang masing-masing cuma
	// punya 1 payment. INV-TAX-SENT (2 payment) sengaja dicek terpisah lewat
	// sentShareSum di bawah, bukan lewat map ini.
	byRef := make(map[string]taxreport.PPh23PaymentRef)
	sentShareSum := decimal.Zero
	for _, p := range summary.PPh23.Payments {
		if p.InvoiceNumber == "INV-TAX-SENT" {
			sentShareSum = sentShareSum.Add(p.PPh23ShareEstimasi)
			continue
		}
		if _, exists := byRef[p.InvoiceNumber]; !exists {
			byRef[p.InvoiceNumber] = p
		}
	}
	assert.True(t, sentShareSum.Equal(decimal.NewFromInt(20)),
		"SUM(share) kedua payment INV-TAX-SENT harus persis sama dengan pph23_estimated_amount invoice itu (20) - got %s", sentShareSum)

	paidPayment := byRef["INV-TAX-PAID"]
	assert.True(t, paidPayment.PPh23ShareEstimasi.Equal(decimal.NewFromInt(10)), "1 payment penuh -> share == pph23_estimated_amount penuh, got %s", paidPayment.PPh23ShareEstimasi)
	assert.Equal(t, "BP-002", *paidPayment.BuktiPotongPPh23Ref)
	assert.Equal(t, "badan_usaha", paidPayment.CustomerType)

	peroranganPayment := byRef["INV-TAX-PERORANGAN"]
	assert.True(t, peroranganPayment.PPh23ShareEstimasi.IsZero(), "customer perorangan -> pph23_estimated_amount invoice-nya 0, share juga harus 0")
	assert.Equal(t, "perorangan", peroranganPayment.CustomerType, "customer_type harus tetap diekspos, BUKAN dipakai untuk memfilter payment ini keluar")
	assert.Nil(t, peroranganPayment.BuktiPotongPPh23Ref)

	draftWithPayment := byRef["INV-TAX-DRAFT-WITH-PAYMENT"]
	assert.True(t, draftWithPayment.PPh23ShareEstimasi.Equal(decimal.NewFromInt(15)),
		"payment terhadap invoice DRAFT tetap harus muncul (pph23.payments tidak memfilter status invoice) - got %s", draftWithPayment.PPh23ShareEstimasi)

	for _, p := range summary.PPh23.Payments {
		assert.NotEqual(t, "INV-TAX-PAYMENT-OUTOFPERIOD", p.InvoiceNumber, "payment yang created_at-nya di luar period harus dikecualikan walau invoice induknya di dalam period")
	}

	require.Len(t, summary.PPN.Invoices, 5)
}

// TestGetSummary_NoDataInPeriod_ReturnsEmptyNotError memastikan endpoint
// tidak error dan tidak panic (mis. divide-by-zero di alokasi proporsional)
// kalau period yang diminta memang kosong sama sekali - kasus wajar untuk
// bulan yang belum ada transaksi apapun.
func TestGetSummary_NoDataInPeriod_ReturnsEmptyNotError(t *testing.T) {
	pool := testhelper.NewPostgresPool(t)
	ctx := context.Background()
	queries := sqlcgen.New(pool)

	repo := taxreport.NewRepository(queries)
	farFuture := time.Now().UTC().AddDate(10, 0, 0).Truncate(24 * time.Hour)
	summary, err := repo.GetSummary(ctx, taxreport.PeriodInput{
		From: farFuture, To: farFuture, ToExclusive: farFuture.AddDate(0, 0, 1),
	})
	require.NoError(t, err)
	assert.Empty(t, summary.PPN.Invoices)
	assert.True(t, summary.PPN.TotalPPNKeluaran.IsZero())
	assert.Empty(t, summary.PPh23.Payments)
	assert.True(t, summary.PPh23.TotalEstimasi.IsZero())
}
