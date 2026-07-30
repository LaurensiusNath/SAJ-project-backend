-- notifications adalah log SEMUA percobaan kirim notifikasi (berhasil
-- maupun gagal) - bukan antrian/outbox (pengiriman terjadi sinkron saat
-- dipanggil, bukan lewat worker terpisah), tapi dicatat di sini supaya
-- kegagalan (SMTP down, dst) kelihatan dan bisa diaudit, bukan cuma hilang
-- di log aplikasi.
--
-- channel cuma mengizinkan 'email' untuk sekarang - WhatsApp (lewat
-- reseller, awalnya direncanakan Fonnte) ditunda karena layanan Fonnte
-- tidak bisa dipakai lagi; provider penggantinya belum diputuskan. Kolom
-- ini sengaja tetap ada (bukan diasumsikan selalu email) supaya penambahan
-- channel baru nanti cuma perlu ALTER TABLE ... CHECK, bukan restrukturisasi.
CREATE TABLE notifications (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    channel VARCHAR(20) NOT NULL CHECK (channel IN ('email')),
    recipient VARCHAR(255) NOT NULL,
    subject VARCHAR(255),
    message TEXT NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('sent', 'failed')),
    error_message TEXT,
    -- job_id/invoice_id nullable & tidak keduanya wajib diisi - notifikasi
    -- "job assigned"/"job completed" terkait job, notifikasi "invoice
    -- terbit" terkait invoice; keduanya cuma untuk penelusuran/audit,
    -- bukan constraint bisnis.
    job_id UUID REFERENCES jobs(id),
    invoice_id UUID REFERENCES invoices(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_notifications_job_id ON notifications(job_id);
CREATE INDEX idx_notifications_invoice_id ON notifications(invoice_id);
