-- name: CreateMachine :one
INSERT INTO machines (customer_id, machine_name, machine_type, serial_number, notes)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListMachinesByCustomer :many
SELECT * FROM machines WHERE customer_id = $1 ORDER BY created_at ASC;
