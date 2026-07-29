-- name: CreateJobCost :one
INSERT INTO job_costs (job_id, cost_type, description, quantity, purchase_price, selling_price)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListJobCostsByJob :many
SELECT * FROM job_costs WHERE job_id = $1 ORDER BY created_at ASC;

-- name: GetJobCostByID :one
SELECT * FROM job_costs WHERE id = $1 AND job_id = $2;

-- name: DeleteJobCost :execrows
DELETE FROM job_costs WHERE id = $1 AND job_id = $2;

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
