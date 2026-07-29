-- Singleton table: id dipaksa selalu 1 lewat CHECK, jadi tidak mungkin ada
-- baris kedua (INSERT dengan id lain gagal CHECK, INSERT dengan id=1 lagi
-- gagal PRIMARY KEY). Constraint ini yang menjamin "singleton", bukan
-- sekadar konvensi di application code.
CREATE TABLE company_settings (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    company_name VARCHAR(255) NOT NULL,
    npwp VARCHAR(30),
    is_pkp BOOLEAN NOT NULL DEFAULT true,
    default_tax_percentage NUMERIC(5, 2) NOT NULL DEFAULT 11 CHECK (default_tax_percentage >= 0),
    default_pph23_rate NUMERIC(5, 2) NOT NULL DEFAULT 2 CHECK (default_pph23_rate >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Baris tunggal ini WAJIB langsung ada (bukan dibuat lewat API) - GET
-- /settings/company harus selalu berhasil, tidak pernah 404 di keadaan awal.
INSERT INTO company_settings (id, company_name, npwp, is_pkp, default_tax_percentage, default_pph23_rate)
VALUES (1, 'PT Servis CNC Sejahtera', '01.234.567.8-901.000', true, 11, 2);
