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

### `GET /jobs/{id}/invoice`
Cek apakah job ini sudah punya invoice (`invoices.job_id` UNIQUE — maksimal satu baris). Response `200` + object Invoice, bentuknya **sama persis** dengan response `POST /jobs/{id}/invoice` (lihat bagian 4). Response `404` kalau job ini belum punya invoice — bukan `200` dengan `data: null`, supaya frontend gampang membedakan "belum di-invoice" dari "request gagal".

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

**Tidak ada state-machine di endpoint ini** — lihat Catatan Desain & Keputusan Teknis Kunci: "`PATCH /invoices/{id}/status` tanpa state-machine/transition guard".

### `GET /invoices`, `GET /invoices/{id}`
`meta: { page, total }` untuk list. Detail invoice: object polos.

`GET /invoices` (list) menambahkan dua field flat di tiap item — `job_code`
dan `customer_name` — hasil JOIN ke `jobs`+`customers`, supaya frontend bisa
menampilkan identitas job/customer yang manusiawi tanpa request tambahan per
baris. **Sengaja flat, bukan nested object job/customer penuh** seperti di
`GET /jobs/{id}` — payload list harus tetap ringan. `GET /invoices/{id}`
(detail) TIDAK mendapat field ini — tetap object `Invoice` polos, cukup
`job_id` mentah (kalau butuh nama customer/job_code, gunakan `GET /jobs/{id}`
dengan `job_id` tersebut).

### `POST /invoices/{id}/payments`
Body: `{ amount (required), payment_method (required, transfer|cash|other), bukti_potong_pph23_ref, notes }`

**Locking**: pessimistic (`SELECT ... FOR UPDATE` pada baris invoice) sebelum baca `SUM(payments.amount)` dan tentukan status baru — mencegah lost-update kalau dua pembayaran masuk nyaris bersamaan. Status jadi `paid` kalau `SUM(payments.amount) + pph23_estimated_amount (jika ada payment dengan bukti_potong_pph23_ref) >= total`.

### `GET /invoices/{id}/payments`
Tanpa pagination, tanpa meta.

### `PATCH /invoices/{id}/payments/{payment_id}/bukti-potong-pph23`
**Role owner/admin saja** (`403` untuk teknisi) — dipasang lewat `requireAdmin` yang disuntik ke `invoice.Handler.RegisterRoutes` (pola sama dengan `PUT /settings/company`), BUKAN via route group terpisah. Semua endpoint LAIN di modul ini tetap terbuka untuk semua role login (`protectedGroup`) — cuma endpoint ini yang dibatasi.

Body: `{ bukti_potong_pph23_ref (required, string) }`

Mengisi/mengganti `bukti_potong_pph23_ref` pada payment yang **sudah ada** — menutup gap: field ini sebelumnya cuma bisa diisi saat payment dibuat (`POST /invoices/{id}/payments`), padahal bukti potong fisik dari customer sering datang belakangan, bukan bersamaan dengan pencatatan pembayaran.

Response `200`: object Payment yang sudah diupdate (bentuk sama dengan response `POST /invoices/{id}/payments`).

Response `404` kalau `payment_id` tidak ditemukan **ATAU** ditemukan tapi bukan milik `invoice_id` di path yang sama (dua kasus ini sengaja dipetakan ke error yang sama, bukan dibedakan — dari sudut pandang client keduanya berarti "kombinasi id ini tidak valid").

**Locking dan re-evaluasi status — WAJIB, bukan opsional**: mengisi bukti potong BUKAN operasi netral — bisa mengubah hasil formula "lunas" yang sama dengan `POST /invoices/{id}/payments` (`SUM(payments.amount) + pph23_estimated_amount (kalau ADA payment dengan bukti_potong_pph23_ref) >= total`), kalau payment ini yang PERTAMA kali membuat invoice ini punya bukti potong. Karena itu endpoint ini mengunci baris invoice yang SAMA (`SELECT ... FOR UPDATE`, query `GetInvoiceForUpdate` yang sama persis dengan `POST /invoices/{id}/payments`) sebelum membaca ulang `SUM(payments.amount)` dan mengevaluasi ulang status — mencegah lost-update kalau ada payment baru masuk nyaris bersamaan dengan bukti potong yang diisi belakangan. Dibuktikan lewat automated test (testcontainers-go, Postgres asli), bukan cuma asumsi — lihat Catatan Desain & Keputusan Teknis Kunci untuk detail lengkap.

**Beda dari `POST /invoices/{id}/payments`**: endpoint itu MENOLAK invoice selain status `draft`/`sent` (`ErrInvoiceNotPayable`, `400`) karena itu menerima UANG BARU. Endpoint ini TIDAK menggerbang status apapun untuk operasi update-nya sendiri (invoice status apapun boleh di-PATCH bukti potongnya — ini cuma mengoreksi dokumen historis, bukan menerima uang baru) — yang dibatasi hanya APAKAH boleh otomatis pindah ke `paid`: hanya kalau status saat ini `draft`/`sent`/`overdue` (masih "berutang" secara aktif). **Sengaja mengizinkan `overdue`** di sini (beda dari gerbang `POST payments`) — salah satu skenario nyata fitur ini justru bukti potong yang telat datang setelah invoice keburu `overdue`. **Sengaja TIDAK mengizinkan `cancelled`** — keputusan bisnis membatalkan invoice tidak boleh diam-diam ditimpa cuma karena dokumen pajak menyusul.

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

## 6. Modul Dashboard

Modul pertama yang murni agregat (bukan CRUD/list-berpaginasi) — baca lintas
`invoices`, `payments`, `jobs`, `customers` sekaligus dalam satu response.

### `GET /dashboard/summary`
**Role owner/admin saja** (`403` untuk teknisi) — dipasang di route group yang
sama dengan `POST /users` (`adminGroup` di `cmd/api/main.go`), modul
`internal/dashboard` sendiri tidak menegakkan otorisasi apapun.

Query opsional: `period_from`, `period_to` (`YYYY-MM-DD`). Kalau salah
satu/keduanya tidak diisi, **masing-masing** default ke awal/akhir bulan
kalender berjalan secara independen — bukan "kalau salah satu diisi, yang
lain ikut menyesuaikan". Kalau cuma `period_from` yang dikirim, `period_to`
tetap jatuh ke akhir bulan berjalan (bisa menghasilkan rentang yang jauh
lebih panjang dari yang mungkin dimaksud client) — sengaja didokumentasikan
di sini karena ini bukan perilaku yang jelas dari nama parameternya saja.
`period_from` setelah `period_to` → `400 VALIDATION_ERROR`.

Response `200`:
```json
{
  "financial": {
    "period": { "from": "2026-08-01", "to": "2026-08-31" },
    "invoiced_total": 3000000, "received_total": 2400000, "outstanding_total": 2000000,
    "by_status": {
      "draft": { "count": 1, "total": 500000 },
      "sent": { "count": 1, "total": 1000000 },
      "paid": { "count": 1, "total": 2000000 },
      "overdue": { "count": 0, "total": 0 },
      "cancelled": { "count": 1, "total": 800000 }
    }
  },
  "jobs": {
    "by_status": { "requested": 3, "scheduled": 1, "in_progress": 1, "completed": 6, "cancelled": 1 },
    "upcoming_7_days": [ { "id": "uuid", "job_code": "JOB-2026-0001", "customer_name": "Bengkel A", "scheduled_date": "2026-08-06" } ],
    "overdue_scheduled": [ { "id": "uuid", "job_code": "JOB-2026-0002", "customer_name": "Pabrik B", "scheduled_date": "2026-08-01" } ]
  }
}
```

**Keputusan/interpretasi yang perlu diketahui frontend:**
- `invoiced_total` = SUM(`invoices.total`) WHERE `created_at` dalam period **DAN status IN (`sent`, `paid`, `overdue`)** — `draft` (belum benar-benar diterbitkan ke customer) dan `cancelled` (sudah dibatalkan) **sengaja tidak ikut terhitung**. Karena itu, `invoiced_total` **selalu** sama dengan `by_status.sent.total + by_status.paid.total + by_status.overdue.total` — kalau frontend butuh reproduksi angka ini secara independen (mis. untuk validasi UI), itu jaminan yang bisa diandalkan, bukan cuma kebetulan.
- `outstanding_total` **TIDAK dibatasi period** (posisi saldo sekarang, bukan arus kas periode) dan **bisa negatif** kalau ada invoice yang di-PATCH manual balik ke status `sent`/`overdue` setelah sempat lunas (lihat Catatan Desain & Keputusan Teknis Kunci: "`PATCH /invoices/{id}/status` tanpa state-machine/transition guard") — sengaja tidak di-clamp ke 0.
- `upcoming_7_days`: `scheduled_date` dari **hari ini sampai +7 hari, inklusif kedua ujung** (8 hari kalender, bukan 7).
- `by_status` financial/jobs SELALU berisi ke-5 key masing-masing walau count-nya 0 — bukan cuma status yang ada datanya.
- `period.from`/`period.to`, sama seperti SEMUA field tanggal-saja lain di kontrak ini (`scheduled_date`, `completed_date`, `due_date`, `upcoming_7_days[].scheduled_date`, `overdue_scheduled[].scheduled_date`), serialize sebagai string `"YYYY-MM-DD"` polos — BUKAN RFC3339. Lihat Catatan Desain & Keputusan Teknis Kunci: "Kenapa field bertipe tanggal sekarang serialize sebagai `YYYY-MM-DD`, bukan RFC3339 penuh?" untuk root cause dan riwayatnya (sempat salah, sudah diperbaiki).

**Transaction boundary** (Atomicity/Consistency, didiskusikan eksplisit sesuai
CLAUDE.md): SATU transaksi **REPEATABLE READ, READ ONLY** membungkus SEMUA
query di endpoint ini (financial + jobs) — lihat penjelasan lengkap kenapa
Read Committed (default) tidak cukup di `internal/dashboard/repository.go`
(`GetSummary`) dan di laporan PR. Endpoint CRUD lain di project ini
sengaja TETAP Read Committed - kebutuhannya berbeda.

---

## 7. Modul Tax Report (Laporan Pajak)

Modul satelit kedua yang murni agregat (setelah Dashboard), untuk halaman
"Laporan Pajak" di frontend — membantu pemilik bisnis mengumpulkan invoice
PPN keluaran dan estimasi bukti potong PPh 23 saat lapor pajak bulanan
(SPT Masa PPN).

### `GET /reports/tax-summary`
**Role owner/admin saja** (`403` untuk teknisi) — dipasang di `adminGroup`
yang sama dengan `GET /dashboard/summary`, modul `internal/taxreport`
sendiri tidak menegakkan otorisasi apapun.

Query opsional: `period_from`, `period_to` (`YYYY-MM-DD`). Default dan
validasi **identik** dengan `GET /dashboard/summary` (masing-masing default
awal/akhir bulan berjalan secara independen; `period_from` setelah
`period_to` → `400 VALIDATION_ERROR`) — lihat Catatan Desain di bawah kenapa
logic ini diduplikasi, bukan di-reuse dari modul Dashboard.

Response `200`:
```json
{
  "period": { "from": "2026-08-01", "to": "2026-08-31" },
  "ppn": {
    "total_ppn_keluaran": 220000,
    "invoices": [
      {
        "id": "uuid", "invoice_number": "INV-2026-0001", "job_code": "JOB-2026-0001",
        "customer_name": "PT Mitra Utama", "subtotal": 1000000, "tax_amount": 110000,
        "nomor_faktur_pajak": "010.000-26.00000001"
      }
    ]
  },
  "pph23": {
    "total_estimasi": 45000,
    "payments": [
      {
        "id": "uuid", "invoice_id": "uuid", "invoice_number": "INV-2026-0001",
        "customer_name": "PT Mitra Utama", "customer_type": "badan_usaha",
        "payment_date": "2026-08-03T10:15:00Z", "pph23_share_estimasi": 12000,
        "bukti_potong_pph23_ref": "BP-001"
      }
    ]
  }
}
```

**Keputusan/interpretasi yang perlu diketahui frontend:**
- `ppn.invoices`: invoice dengan `created_at` dalam period **DAN status IN
  (`sent`, `paid`, `overdue`)** — persis logic `invoiced_total` di Dashboard
  (`draft`/`cancelled` dikecualikan). `total_ppn_keluaran` = SUM(`tax_amount`)
  dari invoice-invoice ini, dihitung dari baris yang sama, jadi selalu
  konsisten dengan daftar `invoices` yang ditampilkan.
- `pph23.payments`: **SEMUA** payment dengan `created_at` dalam period,
  **TIDAK difilter berdasarkan status invoice induknya** — asimetris dengan
  `ppn.invoices` secara sengaja (payment terhadap invoice `draft` tetap
  muncul di sini). Juga **TIDAK difilter** berdasarkan `customer_type` atau
  `bukti_potong_pph23_ref` (null atau tidak) — `customer_type` diekspos apa
  adanya supaya frontend/pengguna yang memutuskan (lihat Catatan Desain:
  keputusan ini belum final).
- `pph23_share_estimasi` per payment adalah alokasi **proporsional** dari
  `pph23_estimated_amount` invoice terhadap seluruh payment invoice itu
  (all-time, bukan cuma yang dalam period) — lihat Catatan Desain untuk rumus
  dan alternatif yang dipertimbangkan lalu ditolak. `total_estimasi` = SUM
  dari kolom ini, bukan query agregat terpisah.
- `payment_date` **RFC3339 penuh** (`payments.created_at`), BUKAN
  `YYYY-MM-DD` — beda dari `period.from`/`period.to` yang tetap date-only.
  Field ini merepresentasikan momen (jam berapa pembayaran dicatat), bukan
  tanggal murni seperti `scheduled_date`/`due_date`, jadi di luar cakupan
  fix date-only-serialization sebelumnya.

**Transaction boundary**: **TIDAK ada transaksi eksplisit** — kedua query
(invoices, payments) jalan langsung lewat connection pool, Read Committed
default. Ini SENGAJA berbeda dari Dashboard (yang wajib REPEATABLE READ) —
lihat Catatan Desain di bawah untuk alasan lengkap.

---

## Catatan Desain & Keputusan Teknis Kunci

Urutan di bawah ini kronologis (kapan ditemukan/diputuskan), bukan alfabetis
atau per-modul — tanggal disertakan kalau memang eksplisit diketahui dari
histori kerja; item lama yang tidak eksplisit bertanggal dibiarkan tanpa
tanggal daripada menebak.

### Kenapa `job_status_history` wajib ada? — 2026-07-30
Audit trail — siapa ubah status apa, kapan. Ini jawaban langsung untuk masalah awal "sering loss informasi". Ditemukan hilang dari implementasi saat audit 2026-07-30, wajib dikembalikan.

### Kenapa nilai uang pakai `shopspring/decimal`, bukan `float64`?
Floating point tidak presisi untuk perhitungan finansial (rounding error di pajak/subtotal berulang bisa akumulasi jadi selisih nyata). Keputusan ini didiskusikan eksplisit saat modul Job Costs dibangun.

### Kenapa `subtotal` di `job_costs` jadi `GENERATED ALWAYS AS ... STORED` column di level database, bukan cuma dihitung di Go?
Menjamin konsistensi di level data itu sendiri — bahkan kalau ada write langsung ke DB di luar aplikasi (migrasi data, query manual), subtotal tidak akan pernah nyasar dari `selling_price * quantity`.

### Kenapa `spare_part` dikecualikan dari dasar PPh 23?
PPh 23 adalah pajak yang dipotong atas jasa (fee/imbalan atas pekerjaan), bukan atas penjualan barang. Komponen `spare_part` di `job_costs` secara substansi adalah transaksi jual-beli barang (perusahaan beli dengan `purchase_price`, jual ke customer dengan markup di `selling_price`), bukan imbalan jasa, jadi tidak termasuk objek PPh 23 yang wajib dipotong customer. `labor` dan `transport` sebaliknya murni komponen jasa. Ini alasan `job_costs` sejak awal dipisah per `cost_type` — bukan cuma untuk margin reporting, tapi supaya `dpp_pph23` bisa dihitung tepat dari subset yang benar secara pajak.

### Kenapa `PATCH /jobs/{id}/assign` pakai optimistic locking, bukan pessimistic seperti di payment?
Kasusnya beda: di payment, kita *mau* request kedua menunggu lalu diproses berurutan (uang tetap harus tercatat semua). Di assign, kita *mau* request kedua **ditolak dan diberi tahu ada konflik** (bukan cuma mengantre lalu diam-diam menimpa) — supaya admin kedua sadar perlu re-check kondisi terbaru sebelum assign ulang.

### Kenapa notifikasi "fire-and-forget"?
Karena notifikasi itu pendukung, bukan sumber kebenaran finansial — kalau email gagal terkirim, itu tidak boleh membatalkan invoice yang sudah sah dibuat. Beda prinsip dengan transaksi finansial yang harus atomic.

### `PATCH /jobs/{id}/status` tanpa state-machine enforcement — 2026-08-01
Transisi status bebas (mis. `completed` → `requested` valid secara backend). Ditemukan saat implementasi frontend. Keputusan: dibiarkan longgar, karena `job_status_history` sudah menjamin audit trail penuh (siapa/kapan), dan `invoices.job_id` UNIQUE sudah mencegah kerugian finansial riil dari status yang salah. Frontend sengaja TIDAK membatasi opsi status di UI (beda dari Invoice) — meniru kelonggaran backend, karena membatasi di UI di sini cuma ilusi keamanan yang bisa dilewati lewat API langsung, beda karakter risiko dari Invoice yang finansial nyata. Diverifikasi ke `internal/job/service.go` (`UpdateStatus`, cuma cek `Status.Valid()`) dan constraint database (cuma keanggotaan enum, tidak ada trigger) — dibuktikan live: `requested → completed` (skip semua tahap), `completed → requested` (mundur total), dan `cancelled → in_progress` (membangkitkan job batal) semuanya diterima `200`.

### `PATCH /invoices/{id}/status` tanpa state-machine/transition guard — 2026-08-02
Diverifikasi langsung terhadap implementasi (`internal/invoice/service.go` `UpdateStatus`, `internal/invoice/domain.go` `Status.Valid()`) dan constraint database (`invoices_status_check`, lihat migration `000014`), keduanya cuma memvalidasi "apakah salah satu dari 5 nilai enum", bukan "apakah transisi dari status saat ini valid". Dibuktikan lewat request nyata terhadap server berjalan: `draft → paid` (skip `sent`), `paid → draft` (mundur), dan `cancelled → sent` (membangkitkan invoice yang sudah dibatalkan) semuanya diterima `200`. **Konsekuensi untuk frontend**: pembatasan opsi manual (mis. cuma menyediakan tombol "Tandai Terkirim"/"Batalkan" di UI) murni tanggung jawab frontend — backend tidak menegakkan apa-apa di luar keanggotaan enum. Kalau nanti butuh state-machine sungguhan, itu perubahan desain baru, bukan bug fix (tidak ada regresi di sini — perilaku ini konsisten sejak `PATCH /invoices/{id}/status` pertama dibuat).

### Kenapa `GET /dashboard/summary` pakai isolation level REPEATABLE READ (read-only), bukan Read Committed (default)? — 2026-08-04
Endpoint ini menjalankan banyak SELECT terpisah (invoiced_total, received_total, outstanding_total, by_status invoice, by_status job, upcoming/overdue job) yang harus tampil sebagai SATU snapshot koheren ke manusia yang melihat dashboard-nya. Read Committed memberi tiap statement snapshot-nya sendiri-sendiri (diambil saat statement itu mulai) - kalau ada write lain (mis. RecordPayment) commit di tengah rangkaian SELECT ini, angka-angka yang tampil di satu response bisa "berasal dari titik waktu berbeda" walau masing-masing individually benar saat dibaca. REPEATABLE READ mengunci SATU snapshot di query pertama transaksi, dipakai seluruh query berikutnya di transaksi yang sama. TIDAK butuh retry logic untuk error `40001` (serialization failure) karena transaksinya read-only (`AccessMode: ReadOnly`) - error itu cuma muncul dari write-write conflict, yang mustahil terjadi di transaksi yang tidak pernah menulis apapun. Lihat `internal/dashboard/repository.go` (`GetSummary`) untuk penjelasan lengkap dan `internal/dashboard/integration_test.go` (`TestGetSummary_RepeatableRead_DoesNotSeeConcurrentCommit`) untuk bukti otomatis (testcontainers-go, Postgres asli) bahwa jaminan ini benar-benar berlaku, bukan cuma asumsi dari nama flag `TxOptions`.

### Kenapa field bertipe tanggal sekarang serialize sebagai `YYYY-MM-DD`, bukan RFC3339 penuh? — 2026-08-05 (perkiraan)
Root cause: `pgtype.Date` (driver Postgres) sebenarnya SUDAH benar sejak awal — dia implement MarshalJSON/UnmarshalJSON sendiri persis format date-only. Masalahnya ada di `pgconv.FromDate`, yang mengonversi `pgtype.Date` ke `*time.Time` polos sebelum sampai ke domain/response — dan `time.Time` default marshal Go SELALU RFC3339 penuh, tanpa peduli komponen waktu nol atau tidak. Fix: tipe baru `internal/dateonly.Date` (value type, bukan driver type — konsisten pola `uuid.UUID`/`decimal.Decimal` yang sudah dipakai langsung di domain struct), `pgconv.ToDate`/`FromDate` diubah untuk konversi ke/dari tipe ini alih-alih `time.Time` polos. Constructor tipe ini WAJIB selalu hasilkan Location=UTC + jam=0 (invariant tegas, diuji eksplisit dengan kasus konstruksi dari zona non-UTC) — mencegah kelas bug pergeseran tanggal ±1 hari yang berbeda dari bug format ini, tapi berpotensi muncul kalau invariant ini dilanggar di masa depan.

Field terdampak: `jobs.scheduled_date`, `jobs.completed_date`, `invoices.due_date`, `dashboard financial.period.from/to`, dan `dashboard upcoming_7_days[].scheduled_date`/`overdue_scheduled[].scheduled_date` (dua yang terakhir ikut kena karena lewat `pgconv.FromDate` yang sama, walau tidak disebut eksplisit di laporan bug awal).

**Request DTO (`createJobRequest.ScheduledDate`, `createInvoiceRequest.DueDate`) SENGAJA TIDAK diikutkan** — sisi input sudah benar sejak awal (`*string` + `time.Parse` manual, bukan lewat `encoding/json` unmarshal `time.Time`), jadi tidak ada bug untuk diperbaiki di situ. Konsekuensinya: fungsi `parseOptionalDate`+`dateLayout` yang terduplikasi 3x (`internal/job/handler.go`, `internal/invoice/handler.go`, `internal/dashboard/handler.go`) TETAP ada, cuma tipe balik `parseOptionalDate` yang berubah supaya nyambung ke domain type baru — lihat item Technical Debt terpisah soal ini.

### Konfirmasi: PPh 23 sudah benar tidak dipotong untuk customer `perorangan` — 2026-08-05
Ditemukan lewat investigasi wajib saat membangun `GET /reports/tax-summary` (bukan bug yang diperbaiki di sini — murni temuan konfirmasi). `ComputeAmounts` (`internal/invoice/domain.go`) sudah menghitung `dpp_pph23`/`pph23_estimated_amount` bergantung `customer_type`: kalau `perorangan`, keduanya di-nolkan; kalau `badan_usaha`, dihitung normal dari `labor`+`transport`. Ini sesuai aturan pajak sebenarnya — PPh 23 dipotong pemberi kerja/badan usaha, bukan orang pribadi. Dibuktikan lewat kode (`internal/invoice/repository.go` baris pemanggilan `ComputeAmounts`) DAN test unit yang sudah ada sebelumnya (`TestComputeAmounts_PeroranganCustomerSkipsPPh23` di `internal/invoice/domain_test.go`) — bukan sesuatu yang baru ditambahkan untuk PR ini. Dicatat di sini supaya tidak perlu diinvestigasi ulang di masa depan.

### `GET /reports/tax-summary`: kenapa `resolvePeriod` diduplikasi lagi dari Dashboard, bukan diekstrak ke package bersama? — 2026-08-05
Dua alasan: (1) loose coupling antar modul bisnis — `dashboard` dan `taxreport` adalah dua modul satelit sejajar, meng-import salah satu dari yang lain cuma untuk satu fungsi kecil melanggar prinsip "modul bisnis tidak saling import langsung antar domain" yang sudah dipegang project ini; (2) konsisten dengan preseden yang sudah diambil project ini untuk kasus persis sama — `parseOptionalDate`+`dateLayout` sudah terduplikasi 3x (job, invoice, dashboard — lihat Technical Debt #10) dan sengaja dibiarkan, bukan diekstrak, supaya tiap PR fitur tetap scoped. Ini sekarang jadi duplikasi ke-2 untuk pola "resolve period dari query param" (dashboard + taxreport) — kalau nanti ada modul KETIGA yang butuh pola sama, itu titik yang tepat untuk benar-benar diekstrak ke package baru (mis. `internal/period`), dicatat sebagai Technical Debt #11.

### Kenapa `GET /reports/tax-summary` pakai Read Committed (default), bukan REPEATABLE READ seperti Dashboard? — 2026-08-05
Berbeda dari Dashboard (yang WAJIB REPEATABLE READ, lihat item di atas), endpoint ini tidak butuh jaminan cross-query: `total_ppn_keluaran` dan `total_estimasi` masing-masing dihitung Go-side dari HASIL query-nya sendiri (bukan query terpisah) — konsisten by construction karena satu statement SQL di Postgres selalu punya snapshot MVCC konsisten sendiri, berlaku di isolation level manapun, bukan cuma REPEATABLE READ. Dan `ppn` (invoice yang diterbitkan) dengan `pph23` (payment yang diterima) adalah dua kategori pajak yang secara substansi berbeda — tidak pernah diharapkan saling cross-foot/rekonsiliasi angka satu sama lain, beda dari Dashboard yang semua angkanya memang dimaksudkan merepresentasikan satu momen yang sama. Konsekuensi: `taxreport.NewRepository` cukup `*sqlcgen.Queries`, tidak butuh `*pgxpool.Pool` sama sekali (tidak ada `pgx.BeginFunc`). Lihat `internal/taxreport/repository.go` (`GetSummary`) untuk penjelasan lengkap.

### Alokasi `pph23_share_estimasi` per payment: proporsional, bukan ditumpuk di payment pertama atau diulang penuh — 2026-08-05
Rumus: `share = pph23_estimated_amount invoice × (payment.amount / total semua payment invoice itu, all-time)`, dibulatkan 2 desimal. Dua alternatif lebih sederhana dipertimbangkan dan ditolak: (1) taruh seluruh `pph23_estimated_amount` di payment PERTAMA invoice, sisanya 0 — salah merepresentasikan kapan kewajiban potong itu timbul, karena bukti potong PPh 23 di dunia nyata biasanya diterbitkan customer PER PEMBAYARAN yang mereka lakukan, proporsional ke jumlah saat itu; (2) tampilkan `pph23_estimated_amount` PENUH di SETIAP payment invoice itu — salah lebih parah, kalau frontend menjumlahkan kolom ini across payments akan double/triple-count. Proporsional adalah satu-satunya opsi di mana SUM(`pph23_share_estimasi`) seluruh payment satu invoice **persis sama** dengan `pph23_estimated_amount` invoice itu (dijamin matematis, modulo pembulatan kecil) — properti yang paling masuk akal untuk direkonsiliasi manual oleh akuntan. **Belum final**: filter berdasarkan `customer_type` (mis. sembunyikan payment dari customer `perorangan` yang share-nya selalu 0) sengaja TIDAK diterapkan di backend — `customer_type` diekspos mentah di tiap payment supaya frontend/pengguna yang memutuskan perlu ditampilkan atau tidak.

### `PATCH /invoices/{id}/payments/{payment_id}/bukti-potong-pph23`: kenapa WAJIB pessimistic locking + re-evaluasi status yang sama dengan `POST /invoices/{id}/payments`? — 2026-08-05
Investigasi wajib sebelum implementasi: apakah mengisi `bukti_potong_pph23_ref` BELAKANGAN (bukan saat payment dibuat) bisa mengubah kondisi "lunas" (`SUM(payments.amount) + pph23_estimated_amount (kalau ADA payment dengan bukti_potong_pph23_ref) >= total`)? **Ya, bisa** — kalau payment yang di-PATCH ini adalah payment PERTAMA pada invoice tersebut yang mencatat bukti potong (`has_bukti_potong` berubah dari `false` ke `true`), `effectivePaid` bisa naik melewati `total` walau `SUM(payments.amount)` (uang yang benar-benar diterima) sama sekali tidak berubah — invoice yang sebelumnya belum `paid` jadi seharusnya `paid` sekarang. Kalau bukti potong sudah ada SEBELUMNYA pada invoice itu (dari payment lain), mengisi/mengganti ref pada payment ini tidak mengubah `has_bukti_potong` (sudah `true`), jadi tidak ada efek ke status — endpoint tetap selalu mengevaluasi ulang tanpa membedakan dua kasus ini secara eksplisit (deteksi "apakah ini flip pertama" tidak diperlukan — evaluasi ulang bersifat idempotent kalau memang tidak ada yang berubah).

Konsekuensinya: endpoint ini **WAJIB** mengunci baris invoice yang SAMA persis dengan `POST /invoices/{id}/payments` (`GetInvoiceForUpdate`, `SELECT ... FOR UPDATE`) sebelum membaca ulang `SUM(payments.amount)`/`has_bukti_potong` dan menentukan status baru — kalau tidak, dua request yang masuk nyaris bersamaan (mis. payment baru lewat `POST` bersamaan dengan bukti potong diisi lewat `PATCH` ini pada payment lain) bisa sama-sama membaca data yang sudah stale sebelum salah satunya commit, persis lost-update yang sama yang sudah dicegah row lock di `POST /invoices/{id}/payments` — cuma pemicunya beda (bukan dua uang baru, tapi satu uang baru + satu metadata pajak yang berubah). Logic penentuan "lunas" diekstrak ke satu fungsi (`computeEffectivePaid` di `internal/invoice/repository.go`) yang dipanggil kedua endpoint, supaya rumusnya tidak diketik ulang dua kali dengan risiko diam-diam melenceng.

Dibuktikan dengan integration test yang sengaja menjalankan `RecordPayment` dan `UpdatePaymentBuktiPotong` **SECARA BERSAMAAN** lewat dua goroutine terhadap invoice yang sama (`TestUpdatePaymentBuktiPotong_ConcurrentWithNewPayment_NoLostUpdate` di `internal/invoice/integration_test.go`, testcontainers-go, Postgres asli) — race ini sengaja melibatkan DUA operasi yang berbeda (bukan `RecordPayment` vs `RecordPayment` seperti test lost-update yang sudah ada), karena row lock harus menyerialkan kedua code path ini terhadap satu sama lain, bukan cuma terhadap dirinya sendiri.

**Beda dari gerbang status di `POST /invoices/{id}/payments`** (`ErrInvoiceNotPayable`, menolak invoice selain `draft`/`sent`): endpoint PATCH ini tidak menggerbang APAKAH operasinya boleh dijalankan berdasarkan status invoice (ini cuma mengoreksi dokumen historis, bukan menerima uang baru — boleh dipanggil untuk invoice status apapun), tapi TETAP membatasi APAKAH boleh otomatis pindah ke `paid`: hanya kalau status saat ini `draft`/`sent`/`overdue`. `overdue` SENGAJA diikutkan di sini (beda dari gerbang `POST payments` yang menolaknya sama sekali) — justru salah satu skenario nyata fitur ini adalah bukti potong yang telat datang setelah invoice sudah keburu `overdue`. `cancelled` SENGAJA dikecualikan — keputusan bisnis membatalkan invoice tidak boleh diam-diam ditimpa jadi `paid` cuma karena dokumen pajak menyusul.

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
| 10 | Duplikasi `parseOptionalDate`+`dateLayout` di 3 handler (job, invoice, dashboard) — Opsi B dari investigasi date-serialization, sengaja ditunda demi jaga PR fix tetap scoped ke output-side saja | 🟢 Rendah, backlog |
| 11 | Duplikasi `resolvePeriod` di 2 modul (dashboard, taxreport) — sama pola dengan #10, ekstrak ke `internal/period` kalau ada modul ketiga yang butuh pola sama | 🟢 Rendah, backlog |
| 12 | `GET /reports/tax-summary` `pph23.payments`: keputusan apakah perlu filter/opsi filter berdasarkan `customer_type` di backend BELUM final — saat ini semua payment diekspos apa adanya, frontend yang memutuskan tampilannya | 🟢 Rendah, butuh keputusan produk |

## Selesai (di luar audit 2026-07-30)

> Tabel ini BARU dibuat di PR date-only-serialization — sebelumnya tidak ada
> tabel "Selesai lainnya" di file ini, dan tidak ada baris/item bernomor 17
> di manapun di `docs/api-contract.md` (dicek eksplisit, bukan asumsi). Kalau
> yang dimaksud "backlog item 17" ada di tempat lain (tracking eksternal,
> percakapan lain), perlu diklarifikasi terpisah — tabel ini cuma menandai
> penyelesaian date-serialization fix sebagai entri baru, bukan memindahkan
> entri yang sudah ada.

| Item | PR |
|---|---|
| Field bertipe tanggal serialize sebagai RFC3339 penuh, bukan `YYYY-MM-DD` | [#20](https://github.com/LaurensiusNath/SAJ-project-backend/pull/20) |
