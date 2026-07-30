-- job_status_history adalah audit trail "siapa mengubah status job apa,
-- kapan, kenapa" - hilang dari implementasi awal (tidak ada di ERD/kontrak
-- yang benar-benar tersimpan sebagai file), ditemukan lewat audit 2026-07-30
-- dan dikembalikan sesuai docs/erd.md v3.
--
-- changed_by NOT NULL karena baris ini HANYA pernah di-insert dari
-- PATCH /jobs/{id}/status, yang selalu melewati RequireAuth (selalu ada
-- user yang terautentikasi) - job.Repository.UpdateStatus menyisipkan
-- baris ini dalam transaksi yang sama dengan UPDATE jobs.status, supaya
-- keduanya konsisten (tidak mungkin status berubah tanpa tercatat, atau
-- tercatat tanpa status benar-benar berubah).
CREATE TABLE job_status_history (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    job_id UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL
        CHECK (status IN ('requested', 'scheduled', 'in_progress', 'completed', 'cancelled')),
    changed_by UUID NOT NULL REFERENCES users(id),
    changed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    notes TEXT
);

CREATE INDEX idx_job_status_history_job_id ON job_status_history(job_id);
