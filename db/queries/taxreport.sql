-- name: TaxSummaryInvoices :many
-- ppn.invoices - SAMA logic exclude draft/cancelled dengan invoiced_total
-- di modul Dashboard (lihat db/queries/dashboard.sql InvoicedTotal): cuma
-- invoice yang sudah benar-benar diterbitkan (bukan draft) dan belum
-- dibatalkan yang jadi objek PPN keluaran. total_ppn_keluaran (SUM tax_amount)
-- dihitung Go-side dari baris yang sama, bukan query SUM terpisah - baris
-- ini juga yang harus ditampilkan penuh ke frontend, jadi tidak ada
-- query kedua yang perlu tetap konsisten dengannya.
SELECT
    invoices.id, invoices.invoice_number, jobs.job_code, customers.name AS customer_name,
    invoices.subtotal, invoices.tax_amount, invoices.nomor_faktur_pajak
FROM invoices
JOIN jobs ON jobs.id = invoices.job_id
JOIN customers ON customers.id = jobs.customer_id
WHERE invoices.status IN ('sent', 'paid', 'overdue')
  AND invoices.created_at >= sqlc.arg(period_from) AND invoices.created_at < sqlc.arg(period_to_exclusive)
ORDER BY invoices.created_at ASC;

-- name: TaxSummaryPayments :many
-- pph23.payments - SEMUA payment dalam period, TANPA filter customer_type
-- (keputusan sengaja belum final - lihat docs/api-contract.md, expose
-- customer_type biar frontend/pengguna yang memutuskan). invoice_total_paid
-- (SUM SEMUA payment invoice itu, ALL-TIME - bukan cuma yang dalam period)
-- dipakai Go-side untuk alokasi proporsional pph23_estimated_amount ke
-- tiap payment kalau satu invoice dicicil lewat beberapa payment - pola
-- LEFT JOIN agregat yang sama dengan OutstandingTotals di dashboard.sql,
-- di sini INNER JOIN karena baris ini pasti punya minimal 1 payment
-- (payments itu sendiri yang sedang di-JOIN).
SELECT
    payments.id, payments.invoice_id, invoices.invoice_number,
    customers.name AS customer_name, customers.customer_type,
    payments.created_at AS payment_date,
    payments.amount AS payment_amount,
    invoices.pph23_estimated_amount,
    payments.bukti_potong_pph23_ref,
    invoice_totals.total_paid AS invoice_total_paid
FROM payments
JOIN invoices ON invoices.id = payments.invoice_id
JOIN jobs ON jobs.id = invoices.job_id
JOIN customers ON customers.id = jobs.customer_id
JOIN (
    SELECT invoice_id, SUM(amount)::numeric AS total_paid
    FROM payments
    GROUP BY invoice_id
) invoice_totals ON invoice_totals.invoice_id = payments.invoice_id
WHERE payments.created_at >= sqlc.arg(period_from) AND payments.created_at < sqlc.arg(period_to_exclusive)
ORDER BY payments.created_at ASC;
