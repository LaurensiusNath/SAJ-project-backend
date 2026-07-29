-- name: CreateJob :one
INSERT INTO jobs (job_code, customer_id, machine_id, title, description, scheduled_date)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: CountJobsByYear :one
-- Dipakai untuk generate job_code (JOB-<tahun>-<urutan>) - lihat repository.go.
-- Cast eksplisit ke ::int, kalau tidak sqlc salah infer tipe parameter
-- jadi pgtype.Timestamptz (ikut tipe kolom created_at) padahal yang
-- dibandingkan hasil EXTRACT() yang berupa angka tahun biasa.
SELECT COUNT(*) FROM jobs WHERE EXTRACT(YEAR FROM created_at)::int = sqlc.arg(year)::int;

-- name: GetJobByID :one
SELECT * FROM jobs WHERE id = $1;

-- name: GetJobForUpdate :one
-- FOR UPDATE mengunci baris job ini sampai transaksi pemanggil
-- commit/rollback - dipakai invoice.Repository.CreateFromJob supaya dua
-- request "generate invoice" untuk job yang sama tidak bisa lolos
-- pengecekan "job belum ada invoice" secara bersamaan (classic
-- check-then-act race). Request kedua akan menunggu di baris ini sampai
-- request pertama selesai, baru membaca status job yang sudah ter-update.
SELECT * FROM jobs WHERE id = $1 FOR UPDATE;

-- name: ListJobs :many
SELECT * FROM jobs
WHERE (status = sqlc.narg('status') OR sqlc.narg('status') IS NULL)
  AND (customer_id = sqlc.narg('customer_id') OR sqlc.narg('customer_id') IS NULL)
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountJobs :one
-- Filter harus selalu sama persis dengan ListJobs, sama seperti pola di
-- db/queries/customers.sql.
SELECT COUNT(*) FROM jobs
WHERE (status = sqlc.narg('status') OR sqlc.narg('status') IS NULL)
  AND (customer_id = sqlc.narg('customer_id') OR sqlc.narg('customer_id') IS NULL);

-- name: UpdateJobStatus :one
UPDATE jobs
SET status = $2, completed_date = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: AssignTechnician :one
UPDATE jobs
SET technician_id = $2, updated_at = now()
WHERE id = $1
RETURNING *;
