CREATE TABLE job_costs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    job_id UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    cost_type VARCHAR(20) NOT NULL
        CHECK (cost_type IN ('labor', 'spare_part', 'transport', 'other')),
    description TEXT NOT NULL,
    quantity NUMERIC(12, 2) NOT NULL CHECK (quantity > 0),
    -- purchase_price cuma masuk akal untuk spare_part (harga beli buat
    -- dibandingkan ke selling_price demi margin) - labor/transport tidak
    -- punya "harga beli".
    purchase_price NUMERIC(14, 2)
        CHECK (cost_type = 'spare_part' OR purchase_price IS NULL),
    selling_price NUMERIC(14, 2) NOT NULL CHECK (selling_price >= 0),
    -- Generated column, bukan dihitung di Go lalu di-insert - subtotal
    -- dijamin selalu = selling_price * quantity oleh database sendiri,
    -- tidak mungkin drift walau ada bug di application code.
    subtotal NUMERIC(14, 2) GENERATED ALWAYS AS (selling_price * quantity) STORED,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_job_costs_job_id ON job_costs(job_id);
