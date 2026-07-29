-- ListInvoices (GET /invoices) selalu ORDER BY created_at DESC - tanpa
-- index ini, Postgres harus Seq Scan + Sort seluruh tabel invoices tiap
-- kali endpoint ini dipanggil. Sama pola dengan idx_customers_created_at/
-- idx_jobs_created_at di migration 000005 - luput ditambahkan saat
-- invoices table dibuat, ketahuan lewat audit EXPLAIN setelahnya.
CREATE INDEX idx_invoices_created_at ON invoices (created_at DESC);
