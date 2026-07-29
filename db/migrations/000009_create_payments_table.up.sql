CREATE TABLE payments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    invoice_id UUID NOT NULL REFERENCES invoices(id),
    amount NUMERIC(14, 2) NOT NULL CHECK (amount > 0),
    payment_method VARCHAR(20) NOT NULL
        CHECK (payment_method IN ('transfer', 'cash', 'other')),
    -- Referensi dokumen bukti potong PPh 23 dari customer (kalau ada) -
    -- bukan angka, cuma catatan dokumentasi, supaya rekonsiliasi tahu kenapa
    -- amount yang diterima lebih kecil dari invoices.total (dipotong pajak,
    -- bukan customer nunggak).
    bukti_potong_pph23_ref VARCHAR(100),
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_payments_invoice_id ON payments(invoice_id);
