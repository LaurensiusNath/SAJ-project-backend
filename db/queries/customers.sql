-- name: CreateCustomer :one
INSERT INTO customers (name, customer_type, phone, email, address, company_name)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetCustomerByID :one
SELECT * FROM customers
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListCustomers :many
SELECT * FROM customers
WHERE deleted_at IS NULL
  AND (name ILIKE '%' || sqlc.narg('search') || '%' OR sqlc.narg('search') IS NULL)
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: UpdateCustomer :one
UPDATE customers
SET name = $2, phone = $3, email = $4, address = $5, company_name = $6, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteCustomer :exec
UPDATE customers SET deleted_at = now() WHERE id = $1;
