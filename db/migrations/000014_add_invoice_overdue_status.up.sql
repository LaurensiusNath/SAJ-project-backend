-- invoices.status kehilangan 'overdue' sebagai state terpisah dari
-- 'cancelled' saat implementasi awal (ditemukan lewat audit 2026-07-30,
-- lihat docs/erd.md/api-contract.md v3) - dikembalikan di sini.
--
-- Postgres tidak punya "ALTER CONSTRAINT", jadi constraint lama harus
-- di-drop lalu dibuat ulang dengan daftar nilai yang sudah termasuk
-- 'overdue'. Nama constraint (invoices_status_check) dikonfirmasi dulu
-- lewat pg_constraint sebelum migration ini ditulis - itu nama default
-- yang di-generate Postgres untuk CHECK constraint kolom tanpa nama
-- eksplisit (pola "<table>_<column>_check").
ALTER TABLE invoices DROP CONSTRAINT invoices_status_check;
ALTER TABLE invoices ADD CONSTRAINT invoices_status_check
    CHECK (status IN ('draft', 'sent', 'paid', 'overdue', 'cancelled'));
