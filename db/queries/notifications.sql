-- name: CreateNotification :one
-- Cuma query yang ada sejauh ini - belum ada endpoint GET untuk riwayat
-- notifikasi (belum diminta di api-contract.md), jadi query List sengaja
-- tidak ditulis dulu (lihat pelajaran GetJobCostByID di db/queries/job_costs.sql:
-- jangan tulis query yang belum ada pemanggilnya). job_id/invoice_id di
-- tabel tetap berguna untuk audit manual lewat psql langsung.
INSERT INTO notifications (channel, recipient, subject, message, status, error_message, job_id, invoice_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;
