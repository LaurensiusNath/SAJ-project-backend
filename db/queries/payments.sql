-- name: CreatePayment :one
INSERT INTO payments (invoice_id, amount, payment_method, bukti_potong_pph23_ref, notes)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListPaymentsByInvoice :many
SELECT * FROM payments WHERE invoice_id = $1 ORDER BY created_at ASC;

-- name: PaymentTotalsByInvoice :one
-- has_bukti_potong menandai apakah SETIDAKNYA satu payment mencatat
-- referensi bukti potong PPh 23 - dipakai RecordPayment untuk memutuskan
-- apakah invoices.pph23_estimated_amount ikut dihitung sebagai "sudah
-- lunas" (lihat service.go untuk penjelasan interpretasi aturan ini).
SELECT
    COALESCE(SUM(amount), 0)::numeric AS total_paid,
    COUNT(*) FILTER (WHERE bukti_potong_pph23_ref IS NOT NULL) > 0 AS has_bukti_potong
FROM payments
WHERE invoice_id = $1;
