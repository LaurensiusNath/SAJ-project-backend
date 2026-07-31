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

-- name: ListJobsNeedingReminderCheck :many
-- Dipakai job.ReminderService (jalan periodik lewat ticker di main.go) -
-- mengambil job yang jadwalnya sudah lewat, hari ini, atau besok, dan
-- statusnya belum final (completed/cancelled). Klasifikasi "overdue vs
-- besok vs hari ini" dilakukan di Go (reminder.go), bukan di sini, supaya
-- teksnya gampang diubah tanpa migration/query baru.
SELECT * FROM jobs
WHERE status NOT IN ('completed', 'cancelled')
  AND scheduled_date IS NOT NULL
  AND scheduled_date <= CURRENT_DATE + INTERVAL '1 day';

-- name: CreateJobStatusHistory :one
-- Dipanggil dari dalam transaksi yang sama dengan UpdateJobStatus (lihat
-- repository.go) - satu baris histori per perubahan status.
INSERT INTO job_status_history (job_id, status, changed_by, notes)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListJobStatusHistory :many
SELECT * FROM job_status_history WHERE job_id = $1 ORDER BY changed_at ASC;

-- name: AssignTechnician :one
-- Optimistic locking: klausa "AND updated_at = $3" cuma berhasil mengubah
-- baris kalau updated_at masih persis sama dengan yang terakhir dibaca
-- client. Kalau ada admin lain yang assign duluan (updated_at sudah
-- berubah), query ini me-return 0 baris (bukan error) - repository.go
-- yang membedakan itu "job tidak ada" vs "job ada tapi datanya sudah usang".
UPDATE jobs
SET technician_id = $2, updated_at = now()
WHERE id = $1 AND updated_at = $3
RETURNING *;
