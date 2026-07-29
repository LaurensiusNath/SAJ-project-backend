CREATE TABLE invoices (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    invoice_number VARCHAR(50) NOT NULL UNIQUE,
    -- Nomor faktur pajak dibuat manual di e-Faktur DJP, di luar sistem ini -
    -- makanya nullable dan diisi belakangan lewat endpoint terpisah
    -- (PATCH /invoices/{id}/faktur-pajak), bukan saat invoice dibuat.
    nomor_faktur_pajak VARCHAR(50),
    -- UNIQUE, bukan cuma index biasa - satu job cuma boleh punya satu
    -- invoice. Ini pagar terakhir di level DB terhadap race condition kalau
    -- dua request "generate invoice" untuk job yang sama lolos dari
    -- pengecekan aplikasi (lihat SELECT ... FOR UPDATE di repository.go).
    job_id UUID NOT NULL UNIQUE REFERENCES jobs(id),
    subtotal NUMERIC(14, 2) NOT NULL,
    tax_percentage NUMERIC(5, 2) NOT NULL,
    tax_amount NUMERIC(14, 2) NOT NULL,
    total NUMERIC(14, 2) NOT NULL,
    dpp_pph23 NUMERIC(14, 2) NOT NULL DEFAULT 0,
    -- pph23_rate disimpan (bukan cuma pph23_estimated_amount) supaya
    -- perhitungan tetap bisa direkonstruksi/diaudit walau
    -- company_settings.default_pph23_rate berubah di masa depan - sama
    -- alasannya dengan tax_percentage di atas.
    pph23_rate NUMERIC(5, 2) NOT NULL DEFAULT 0,
    pph23_estimated_amount NUMERIC(14, 2) NOT NULL DEFAULT 0,
    expected_receivable NUMERIC(14, 2) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'sent', 'paid', 'cancelled')),
    due_date DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_invoices_status ON invoices(status);
