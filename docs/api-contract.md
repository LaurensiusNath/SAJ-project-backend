# API Contract – CNC Service Project Management App
### v3 – disinkronkan dengan hasil audit implementasi (2026-07-30)
### Perubahan dari v2: tambah modul Auth/User, Notification; perbaiki status invoice;
### wajibkan nested data di GET /jobs/{id}; dokumentasikan bentuk `meta` per endpoint
### Update 2026-07-31: strategi Auth berubah dari Bearer-token-only ke httpOnly cookie
### (+ Bearer tetap didukung sebagai fallback), hasil diskusi strategi arsitektur frontend
### (Next.js same-origin lewat proxy). Endpoint `POST /auth/logout` ditambahkan.

**Base URL**: `/api/v1` (kecuali `/health` dan `/auth/login`, publik tanpa prefix/token)
**Auth**: JWT (HS256), dikirim lewat httpOnly cookie `access_token` (jalur utama, dipakai frontend Next.js) atau header `Authorization: Bearer <token>` (fallback — testing manual/Postman, tooling, kemungkinan client non-browser). Cookie diprioritaskan kalau keduanya dikirim. Role: `owner`, `admin`, `teknisi`.
**Format response standar**:
```json
// Success
{ "success": true, "data": { ... }, "meta": { ... } }
// Error
{ "success": false, "error": { "code": "...", "message": "..." } }
```
> Catatan bentuk `meta`: **sengaja tidak seragam**, disesuaikan kebutuhan tiap endpoint —
> lihat catatan per-endpoint di bawah. Ini keputusan sadar, bukan inkonsistensi yang perlu diperbaiki.

---

## 0. Modul Auth & User

### `POST /auth/login` – publik, tanpa token
Body: `{ "email": "...", "password": "..." }`
Response `200`: set cookie `access_token` (`HttpOnly`, `SameSite=Lax`, `Secure` di production saja, `Path=/`, `Max-Age` sesuai masa berlaku JWT). Body response: `{ "user": { "id", "name", "email", "role" } }` — **tidak lagi** mengembalikan token mentah di JSON (kalau dikembalikan juga, tujuan httpOnly jadi percuma karena JS bisa baca dari response body).
Response `401`: pesan identik untuk email tidak ditemukan ATAU password salah (sengaja, untuk tidak membocorkan mana yang salah).

### `POST /auth/logout` – baru, sebelumnya tidak ada di kontrak sama sekali
Butuh token valid (cookie/Bearer). Response `200`: set cookie `access_token` dengan `Max-Age=0` (hapus cookie di browser).

### Auth Middleware – `RequireAuth`
Dual support: baca token dari cookie `access_token` (jalur utama untuk frontend Next.js) atau header `Authorization: Bearer <token>` (tetap didukung untuk keperluan lain — testing manual, tooling, kemungkinan client non-browser di masa depan). Cookie diprioritaskan kalau keduanya ada.

> **Catatan arsitektur (2026-07-31)**: keputusan pakai httpOnly cookie mengharuskan frontend & backend diakses dari origin yang sama — development pakai Next.js rewrites (`/api/*` → backend lokal), production pakai reverse proxy (Nginx/Caddy) di bawah satu domain. Ini alasan kenapa axios `baseURL` di frontend cukup `/api` (relative), tidak perlu env var URL backend absolut untuk request dari browser.

### `POST /users` – role `owner`/`admin`
Body: `{ "name", "email", "password" (min 8 char), "role" (owner|admin|teknisi) }`
Response `201`: object User **tanpa** `password_hash`.

### `GET /users` – role `owner`/`admin` – baru, menutup blocker assign-teknisi
Query: `role` (opsional, filter — dipakai frontend untuk dropdown assign teknisi: `?role=teknisi`)
Response `200`: array User (`id`, `name`, `email`, `role`, `created_at`) **tanpa** `password_hash`. **Tanpa pagination** (jumlah user diasumsikan selalu kecil, sama seperti Machines).

### `GET /auth/me` – semua role yang login – baru, menutup blocker info user setelah refresh
Tanpa body/query.
Response `200`: `{ "user": { "id", "name", "email", "role" } }` — bentuk **sama persis** dengan body `POST /auth/login`, sumber dari token yang sedang aktif (cookie/Bearer).

> **Backlog**: `PUT /users/{id}`, `DELETE /users/{id}` masih belum ada — belum ada kebutuhan konkret untuk itu.

---

## 1. Modul Company Settings

### `GET /settings/company` – semua role yang login
Response `200`: `{ company_name, npwp, is_pkp, default_tax_percentage, default_pph23_rate, updated_at }`

### `PUT /settings/company` – role `owner`/`admin`
Body: `{ company_name (required), npwp, is_pkp, default_tax_percentage (required), default_pph23_rate (required) }`

---

## 2. Modul Customer

### `POST /customers`
Body: `{ name (required), customer_type (required, badan_usaha|perorangan), phone, email, address, company_name }`
Response `201`: object Customer.

### `GET /customers`
Query: `page` (default 1), `limit` (default 20), `search`, `customer_type`
`meta: { page, total }`

### `GET /customers/{id}`
Response `200`: object Customer + field `machines: []Machine` (nested).

### `PUT /customers/{id}`
Body: `{ name (required), phone, email, address, company_name }`
**`customer_type` tidak bisa diubah lewat endpoint ini** (sengaja — customer_type menentukan kewajiban PPh 23, perubahan tipe customer setelah job berjalan berisiko bikin invoice lama & baru tidak konsisten secara pajak). Kalau memang perlu diubah, harus lewat proses manual/lain yang belum didefinisikan — **item backlog untuk didiskusikan**.

### `DELETE /customers/{id}`
Soft delete. Response `204`.

### Sub-resource: Machines
`POST /customers/{id}/machines` – body: `{ machine_name (required), machine_type, serial_number, notes }`
`GET /customers/{id}/machines` – **tanpa pagination, tanpa meta** (jumlah mesin per customer diasumsikan selalu kecil).

---

## 3. Modul Job (Work Order)

### `POST /jobs`
Body: `{ customer_id (required), machine_id, title (required), description, scheduled_date ("YYYY-MM-DD") }`
`job_code` di-generate backend (`JOB-<tahun>-<urutan>`), tidak bisa diisi client. Status awal selalu `requested`.

### `GET /jobs`
Query: `page`, `limit`, `status`, `customer_id` – `meta: { page, total }`

### `GET /jobs/{id}` – **wajib nested, ini requirement, bukan opsional**
Response `200` harus berisi object Job **plus**:
- `status_history: []JobStatusHistory` (urut kronologis)
- `costs: []JobCost`

> Kalau implementasi saat ini mengembalikan object polos tanpa dua field ini, itu perlu diperbaiki — frontend detail-job butuh ini dalam satu request, bukan 3x round-trip.

### `PATCH /jobs/{id}/status`
Body: `{ status (required, requested|scheduled|in_progress|completed|cancelled), notes }`
`completed_date` otomatis diisi/dikosongkan mengikuti status. **Setiap perubahan wajib insert row baru ke `job_status_history`** (`status`, `changed_by` dari token JWT, `changed_at`, `notes`).

### `PATCH /jobs/{id}/assign`
Body: `{ technician_id (required), expected_updated_at (required, RFC3339) }`
**Optimistic locking**: `expected_updated_at` dicocokkan ke `jobs.updated_at` saat ini di dalam `WHERE` clause update. Kalau tidak cocok (sudah diubah request lain) — `409 CONFLICT`. Dipilih di atas pessimistic locking secara sadar, karena pessimistic cuma menyerialkan urutan tulis (tetap silent-overwrite), sedangkan optimistic mendeteksi & menolak konfliknya secara eksplisit.

### Sub-resource: Job Costs
`POST /jobs/{id}/costs` – body: `{ cost_type (required, labor|spare_part|transport|other), description (required), quantity (required), purchase_price, selling_price (required) }`. `purchase_price` ditolak `400` kalau `cost_type != spare_part`. `subtotal` generated column, tidak bisa diisi client.
`GET /jobs/{id}/costs` – tanpa pagination. `meta: { total_selling, total_margin }` (bentuk khusus, beda dari list lain — sengaja, karena kebutuhannya beda: total buat subtotal invoice, margin buat insight, bukan buat navigasi halaman).
`DELETE /jobs/{id}/costs/{cost_id}` – `204`.

---

## 4. Modul Costing / Invoice

### `POST /jobs/{id}/invoice`
Body: `{ tax_percentage (opsional, default dari company_settings), due_date (opsional) }`
Response `400` kalau job belum `completed` atau job sudah pernah punya invoice (`job_id` UNIQUE — **satu job maksimal satu invoice, dikonfirmasi memang aturan bisnis**; pembayaran bertahap ditangani lewat multiple `payments` per invoice, bukan multiple invoice per job).

**Transaction boundary** (penting, ini contoh Atomicity yang didiskusikan eksplisit): satu transaksi membungkus kunci baris job (`SELECT FOR UPDATE`), baca job+job_costs+customer+company_settings, hitung, insert invoice. Isolation level default (Read Committed) — konsistensi dijamin lewat row lock eksplisit, bukan snapshot level transaksi.

Response `201`:
```json
{
  "id": "uuid", "invoice_number": "INV-2026-0042", "nomor_faktur_pajak": null,
  "job_id": "uuid", "subtotal": 1500000, "tax_percentage": 11, "tax_amount": 165000,
  "total": 1665000, "dpp_pph23": 800000, "pph23_rate": 2, "pph23_estimated_amount": 16000,
  "expected_receivable": 1649000, "status": "draft", "due_date": "2026-08-15",
  "created_at": "2026-07-28T10:00:00Z"
}
```

### `PATCH /invoices/{id}/faktur-pajak`
Body: `{ nomor_faktur_pajak (required) }`

### `PATCH /invoices/{id}/status`
Body: `{ status (required, draft|sent|paid|overdue|cancelled) }`
**`overdue` sebaiknya di-set otomatis**, bukan cuma manual: perluas `ReminderService` (ticker per jam yang sudah ada untuk reminder jadwal) supaya juga menandai invoice `sent` yang `due_date`-nya sudah lewat dan belum lunas jadi `overdue`. `cancelled` tetap manual (keputusan sengaja membatalkan invoice).

### `GET /invoices`, `GET /invoices/{id}`
`meta: { page, total }` untuk list. Detail invoice: object polos.

### `POST /invoices/{id}/payments`
Body: `{ amount (required), payment_method (required, transfer|cash|other), bukti_potong_pph23_ref, notes }`

**Locking**: pessimistic (`SELECT ... FOR UPDATE` pada baris invoice) sebelum baca `SUM(payments.amount)` dan tentukan status baru — mencegah lost-update kalau dua pembayaran masuk nyaris bersamaan. Status jadi `paid` kalau `SUM(payments.amount) + pph23_estimated_amount (jika ada payment dengan bukti_potong_pph23_ref) >= total`.

### `GET /invoices/{id}/payments`
Tanpa pagination, tanpa meta.

---

## 5. Modul Notification (baru – tidak ada di kontrak/ERD awal)

Tidak ada endpoint HTTP publik untuk modul ini saat ini — murni internal, dipicu dari modul lain:
- Job di-assign teknisi → email ke customer
- Job jadi `completed` → email ke customer
- Invoice diterbitkan → email invoice ke customer
- `ReminderService` (goroutine + ticker 1 jam) — cek `scheduled_date` job yang lewat/hari-ini/besok, kirim reminder. Idempotency dijamin lewat cek "sudah pernah kirim reminder jenis ini hari ini" di tabel `notifications`, bukan state di memori.

**Channel saat ini cuma `email` (SMTP)**. WhatsApp (rencana awal pakai Fonnte) ditunda — Fonnte sudah tidak bisa dipakai, pengganti (Wablas / WhatsApp Cloud API resmi) **belum diputuskan, backlog**.

**Desain "fire-and-forget"**: kegagalan kirim notifikasi (SMTP down, dst) **tidak pernah** menggagalkan operasi bisnis yang memicunya — dicatat di `notifications.status = 'failed'` saja. Ini trade-off sadar (eventual/best-effort di atas strict consistency untuk domain notifikasi), berbeda dari domain finansial yang strict.

> **Backlog**: belum ada `GET /jobs/{id}/notifications` atau semacamnya untuk lihat riwayat notifikasi dari sisi frontend/admin. Belum dibutuhkan konkret, tapi datanya sudah tersimpan (`job_id`/`invoice_id` di tabel), tinggal dibuatkan endpoint kalau perlu.

---

## Catatan Desain & Keputusan Teknis Kunci

1. **Kenapa `job_status_history` wajib ada?** Audit trail — siapa ubah status apa, kapan. Ini jawaban langsung untuk masalah awal "sering loss informasi". **(Ditemukan hilang dari implementasi saat audit 2026-07-30, wajib dikembalikan.)**
2. **Kenapa nilai uang pakai `shopspring/decimal`, bukan `float64`?** Floating point tidak presisi untuk perhitungan finansial (rounding error di pajak/subtotal berulang bisa akumulasi jadi selisih nyata). Keputusan ini didiskusikan eksplisit saat modul Job Costs dibangun.
3. **Kenapa `subtotal` di `job_costs` jadi `GENERATED ALWAYS AS ... STORED` column di level database, bukan cuma dihitung di Go?** Menjamin konsistensi di level data itu sendiri — bahkan kalau ada write langsung ke DB di luar aplikasi (migrasi data, query manual), subtotal tidak akan pernah nyasar dari `selling_price * quantity`.
4. **Kenapa `PATCH /jobs/{id}/assign` pakai optimistic locking, bukan pessimistic seperti di payment?** Kasusnya beda: di payment, kita *mau* request kedua menunggu lalu diproses berurutan (uang tetap harus tercatat semua). Di assign, kita *mau* request kedua **ditolak dan diberi tahu ada konflik** (bukan cuma mengantre lalu diam-diam menimpa) — supaya admin kedua sadar perlu re-check kondisi terbaru sebelum assign ulang.
5. **Kenapa notifikasi "fire-and-forget"?** Karena notifikasi itu pendukung, bukan sumber kebenaran finansial — kalau email gagal terkirim, itu tidak boleh membatalkan invoice yang sudah sah dibuat. Beda prinsip dengan transaksi finansial yang harus atomic.

---

## Technical Debt / Prioritas Perbaikan (dari audit 2026-07-30)

| # | Item | Prioritas |
|---|---|---|
| 1 | `job_status_history` tidak ada, harus dibuat migration baru + wiring di service | 🔴 Kritis |
| 2 | Dockerfile tidak menjalankan `sqlc generate`; `docker-compose.yml` tidak menjalankan migration sebelum start app | 🔴 Kritis |
| 3 | Nol automated test untuk transaction/locking (bagian paling ACID-critical) — perlu `testcontainers-go`, bukan script manual yang dihapus | 🟠 Tinggi |
| 4 | `GET /jobs/{id}` belum nested `status_history`+`costs` sesuai kontrak | 🟠 Tinggi |
| 5 | `invoices.status` perlu tambah kembali `overdue` sebagai state terpisah dari `cancelled` + auto-transition via `ReminderService` | 🟡 Sedang |
| 6 | `go.mod`: `golang-jwt/jwt/v5` salah ditandai `// indirect`, perlu `go mod tidy` yang bersih (hati-hati toolchain jangan ke-upgrade otomatis ke 1.25) | 🟢 Rendah |
| 7 | CRUD User belum lengkap (`GET/PUT/DELETE /users/{id}`) | 🟢 Rendah, backlog |
| 8 | Provider WhatsApp pengganti Fonnte belum diputuskan | 🟢 Rendah, backlog |
| 9 | HS256 vs RS256 untuk persiapan microservice extraction | 🟢 Rendah, backlog, revisit saat ekstraksi Notification Service |
