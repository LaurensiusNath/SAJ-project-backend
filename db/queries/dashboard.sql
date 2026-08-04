-- Semua query di file ini SELALU dipanggil di dalam SATU transaksi
-- REPEATABLE READ read-only (lihat internal/dashboard/repository.go) - tidak
-- ada satupun yang boleh dipanggil berdiri sendiri di luar transaksi itu,
-- karena tujuannya justru supaya semuanya melihat snapshot data yang sama.

-- name: InvoicedTotal :one
-- period_from/period_to_exclusive: batas [from, to) - to_exclusive sudah
-- "besok dari period_to yang diminta user" supaya seluruh hari terakhir
-- period ikut ter-hitung (lihat Service untuk perhitungan tanggalnya).
--
-- status IN (sent, paid, overdue) - SENGAJA exclude draft dan cancelled.
-- "invoiced" berarti sudah benar-benar diterbitkan ke customer (draft masih
-- internal, belum tentu jadi) dan jelas bukan yang sudah dibatalkan. Angka
-- ini HARUS selalu sama dengan
-- (by_status.sent.total + by_status.paid.total + by_status.overdue.total)
-- - lihat TestGetSummary_InvoicedTotal_EqualsSentPlusPaidPlusOverdue di
-- integration_test.go, jaring pengaman kalau logic ini berubah lagi nanti.
SELECT COALESCE(SUM(total), 0)::numeric AS total
FROM invoices
WHERE created_at >= sqlc.arg(period_from) AND created_at < sqlc.arg(period_to_exclusive)
  AND status IN ('sent', 'paid', 'overdue');

-- name: ReceivedTotal :one
SELECT COALESCE(SUM(amount), 0)::numeric AS total
FROM payments
WHERE created_at >= sqlc.arg(period_from) AND created_at < sqlc.arg(period_to_exclusive);

-- name: OutstandingTotals :one
-- TIDAK dibatasi period (lihat api-contract.md: ini saldo posisi sekarang,
-- bukan arus periode). "paid" di sini menjumlah SEMUA payment yang pernah
-- masuk untuk invoice berstatus sent/overdue itu, terlepas kapan payment-nya
-- dicatat - LEFT JOIN supaya invoice yang belum ada payment sama sekali
-- tetap ikut (paid = 0), bukan hilang dari agregat.
SELECT
    COALESCE(SUM(i.total), 0)::numeric AS invoiced,
    COALESCE(SUM(paid.amount), 0)::numeric AS paid
FROM invoices i
LEFT JOIN (
    SELECT invoice_id, SUM(amount) AS amount
    FROM payments
    GROUP BY invoice_id
) paid ON paid.invoice_id = i.id
WHERE i.status IN ('sent', 'overdue');

-- name: InvoiceCountsByStatus :many
-- Cuma mengembalikan baris untuk status yang benar-benar punya invoice di
-- period ini (GROUP BY tidak menghasilkan baris kosong) - Service yang
-- mengisi status lain dengan count/total 0 supaya response selalu punya
-- ke-5 key sesuai kontrak.
SELECT status, COUNT(*) AS count, COALESCE(SUM(total), 0)::numeric AS total
FROM invoices
WHERE created_at >= sqlc.arg(period_from) AND created_at < sqlc.arg(period_to_exclusive)
GROUP BY status;

-- name: JobCountsByStatus :many
-- ALL-TIME, sengaja tanpa filter period (ini snapshot kondisi sekarang,
-- bukan arus - lihat api-contract.md).
SELECT status, COUNT(*) AS count
FROM jobs
GROUP BY status;

-- name: UpcomingScheduledJobs :many
-- "hari ini s/d +7 hari" diinterpretasikan inklusif kedua ujung (8 hari
-- kalender total: hari ini + 7 hari ke depan) - lihat penjelasan di laporan
-- kenapa ini butuh ditulis eksplisit (rawan off-by-one).
SELECT jobs.id, jobs.job_code, customers.name AS customer_name, jobs.scheduled_date
FROM jobs
JOIN customers ON customers.id = jobs.customer_id
WHERE jobs.scheduled_date BETWEEN CURRENT_DATE AND (CURRENT_DATE + INTERVAL '7 days')
  AND jobs.status IN ('requested', 'scheduled', 'in_progress')
ORDER BY jobs.scheduled_date ASC;

-- name: OverdueScheduledJobs :many
SELECT jobs.id, jobs.job_code, customers.name AS customer_name, jobs.scheduled_date
FROM jobs
JOIN customers ON customers.id = jobs.customer_id
WHERE jobs.scheduled_date < CURRENT_DATE
  AND jobs.status NOT IN ('completed', 'cancelled')
ORDER BY jobs.scheduled_date ASC;
