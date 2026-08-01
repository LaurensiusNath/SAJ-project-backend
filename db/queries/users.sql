-- name: CreateUser :one
INSERT INTO users (name, email, password_hash, role)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1 AND deleted_at IS NULL;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1 AND deleted_at IS NULL;

-- name: ListUsers :many
-- Tanpa LIMIT/OFFSET (tanpa pagination) - jumlah akun staf diasumsikan
-- selalu kecil, pola yang sama dengan ListMachinesByCustomer.
SELECT * FROM users
WHERE deleted_at IS NULL
  AND (role = sqlc.narg('role') OR sqlc.narg('role') IS NULL)
ORDER BY created_at DESC;
