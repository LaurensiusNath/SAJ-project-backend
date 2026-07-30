-- name: CreateNotification :one
-- Belum ada endpoint GET untuk riwayat notifikasi (belum diminta di
-- api-contract.md) - job_id/invoice_id tetap berguna untuk audit manual
-- lewat psql langsung (lihat juga pelajaran GetJobCostByID di
-- db/queries/job_costs.sql: jangan tulis query yang belum ada pemanggilnya -
-- ExistsNotificationSentToday di bawah ADA pemanggilnya, job.ReminderService).
INSERT INTO notifications (channel, recipient, subject, message, status, error_message, job_id, invoice_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ExistsNotificationSentToday :one
-- Idempotency check dipakai job.ReminderService - reminder cuma boleh
-- terkirim SEKALI per job per jenis (dibedakan lewat subject) per hari,
-- walau ticker-nya sendiri jalan tiap jam (lihat main.go). Tanpa ini,
-- job yang masih overdue akan di-spam email tiap kali ticker jalan.
SELECT EXISTS (
    SELECT 1 FROM notifications
    WHERE job_id = $1 AND subject = $2 AND created_at::date = CURRENT_DATE
) AS already_sent;
