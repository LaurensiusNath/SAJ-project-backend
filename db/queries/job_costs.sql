-- name: CreateJobCost :one
INSERT INTO job_costs (job_id, cost_type, description, quantity, purchase_price, selling_price)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListJobCostsByJob :many
SELECT * FROM job_costs WHERE job_id = $1 ORDER BY created_at ASC;

-- name: DeleteJobCost :execrows
DELETE FROM job_costs WHERE id = $1 AND job_id = $2;

-- name: JobCostInvoiceTotals :one
-- Dipakai invoice.Repository.CreateFromJob (bukan JobCostTotals di atas -
-- itu untuk meta.total_selling/total_margin di GET /jobs/{id}/costs, beda
-- kebutuhan). subtotal_all = dasar PPN (semua cost_type). dpp_pph23 = dasar
-- PPh 23, HANYA cost_type jasa (labor, transport) - spare_part sengaja
-- dikecualikan sesuai aturan pajak (lihat api-contract.md Catatan Desain #2).
SELECT
    COALESCE(SUM(subtotal), 0)::numeric AS subtotal_all,
    COALESCE(SUM(subtotal) FILTER (WHERE cost_type IN ('labor', 'transport')), 0)::numeric AS dpp_pph23
FROM job_costs
WHERE job_id = $1;

-- name: JobCostTotals :one
-- total_selling = dasar subtotal invoice nanti (SUM semua cost_type).
-- total_margin = cuma dari spare_part (selling_price - purchase_price) x quantity,
-- sesuai definisi margin di api-contract.md.
SELECT
    COALESCE(SUM(subtotal), 0)::numeric AS total_selling,
    COALESCE(SUM(
        CASE WHEN cost_type = 'spare_part' THEN (selling_price - purchase_price) * quantity ELSE 0 END
    ), 0)::numeric AS total_margin
FROM job_costs
WHERE job_id = $1;
