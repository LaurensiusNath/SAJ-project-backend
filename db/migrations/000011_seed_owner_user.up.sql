-- Seed satu akun owner awal - sama pola dengan company_settings singleton
-- (migration 000007): tidak ada endpoint registrasi publik sama sekali
-- (lihat api-contract.md), jadi user PERTAMA harus ada sebelum siapapun
-- bisa login dan memakai POST /users untuk mendaftarkan admin/teknisi lain.
--
-- Password default: "ChangeMe123!" - hash di bawah dihasilkan sekali lewat
-- bcrypt.GenerateFromPassword (cost 10). WAJIB diganti (lewat mekanisme
-- ganti password saat itu dibuat, atau langsung UPDATE manual) sebelum
-- deployment sungguhan - ini cuma buat development/testing.
INSERT INTO users (name, email, password_hash, role)
VALUES (
    'Pemilik Bengkel',
    'owner@cncservis.local',
    '$2a$10$Ade3V4SvpZlboEVWZBqmweB6yEU6fp9IBS8dRllALJEimWmrtWlWO',
    'owner'
);
