-- name: CountInvoicesByYear :one
-- Dipakai untuk generate invoice_number (INV-<tahun>-<urutan>), pola sama
-- persis dengan CountJobsByYear di jobs.sql - termasuk celah race condition
-- kecil yang sama (diterima di skala project ini, lihat repository.go).
SELECT COUNT(*) FROM invoices WHERE EXTRACT(YEAR FROM created_at)::int = sqlc.arg(year)::int;

-- name: CreateInvoice :one
INSERT INTO invoices (
    invoice_number, job_id, subtotal, tax_percentage, tax_amount, total,
    dpp_pph23, pph23_rate, pph23_estimated_amount, expected_receivable, due_date
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)
RETURNING *;

-- name: GetInvoiceByID :one
SELECT * FROM invoices WHERE id = $1;

-- name: GetInvoiceByJobID :one
-- Dipakai untuk pengecekan "job ini sudah punya invoice belum" di dalam
-- transaksi CreateFromJob (setelah GetJobForUpdate mengunci baris job-nya).
SELECT * FROM invoices WHERE job_id = $1;

-- name: GetInvoiceForUpdate :one
-- FOR UPDATE mengunci baris invoice ini - dipakai RecordPayment supaya dua
-- pembayaran yang masuk nyaris bersamaan tidak sama-sama menghitung SUM
-- pembayaran dari data yang stale (lost update pada invoices.status).
SELECT * FROM invoices WHERE id = $1 FOR UPDATE;

-- name: ListInvoices :many
-- JOIN ke jobs+customers cuma untuk dua kolom flat (job_code, customer_name) -
-- list endpoint butuh identitas yang manusiawi tanpa request tambahan dari
-- frontend, tapi sengaja TIDAK nested object penuh seperti GetInvoiceByID/
-- detail (lihat domain.go InvoiceListItem) supaya payload list tetap ringan.
SELECT invoices.*, jobs.job_code, customers.name AS customer_name
FROM invoices
JOIN jobs ON jobs.id = invoices.job_id
JOIN customers ON customers.id = jobs.customer_id
WHERE (invoices.status = sqlc.narg('status') OR sqlc.narg('status') IS NULL)
ORDER BY invoices.created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountInvoices :one
SELECT COUNT(*) FROM invoices
WHERE (status = sqlc.narg('status') OR sqlc.narg('status') IS NULL);

-- name: UpdateInvoiceFakturPajak :one
UPDATE invoices
SET nomor_faktur_pajak = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateInvoiceStatus :one
UPDATE invoices
SET status = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: MarkOverdueInvoices :many
-- Dipanggil dari ticker reminder yang sama dengan job.ReminderService
-- (lihat cmd/api/main.go/runReminderScheduler) - hanya invoice 'sent' yang
-- due_date-nya sudah lewat HARI INI (bukan hari ini sendiri) yang
-- ditandai overdue. Invoice 'paid'/'draft'/'cancelled'/yang sudah
-- 'overdue' tidak tersentuh (idempotent - aman dipanggil berkali-kali).
UPDATE invoices
SET status = 'overdue', updated_at = now()
WHERE status = 'sent' AND due_date < CURRENT_DATE
RETURNING *;
