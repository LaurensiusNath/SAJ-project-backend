-- Turun balik: gagal kalau ada baris berstatus 'overdue' saat ini (sengaja
-- dibiarkan gagal, bukan diam-diam diubah jadi status lain - keputusan itu
-- harus eksplisit, bukan efek samping rollback).
ALTER TABLE invoices DROP CONSTRAINT invoices_status_check;
ALTER TABLE invoices ADD CONSTRAINT invoices_status_check
    CHECK (status IN ('draft', 'sent', 'paid', 'cancelled'));
