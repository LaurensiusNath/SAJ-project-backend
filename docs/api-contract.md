# API Contract – CNC Service Project Management App
### Modul: Customer, Job, Costing/Invoice, Company Settings
### v2 – sudah disesuaikan dengan aturan PPN & PPh 23 (PT sudah PKP, spare part ada markup)

**Base URL**: `/api/v1`
**Auth**: Semua endpoint (kecuali `/auth/login`) butuh header `Authorization: Bearer <access_token>`.
**Format response standar**:

```json
// Success
{ "success": true, "data": { ... }, "meta": { "page": 1, "total": 42 } }

// Error
{ "success": false, "error": { "code": "VALIDATION_ERROR", "message": "..." } }
```

---

## 0. Modul Company Settings

Karena PT sudah PKP, konfigurasi pajak dipusatkan di sini – bukan diinput ulang tiap invoice, supaya konsisten dan gampang diubah kalau aturan pajak berubah lagi.

### `GET /settings/company`
Ambil konfigurasi perusahaan (singleton, cuma 1 row).

Response `200`:
```json
{
  "success": true,
  "data": {
    "company_name": "PT Servis CNC Sejahtera",
    "npwp": "01.234.567.8-901.000",
    "is_pkp": true,
    "default_tax_percentage": 11,
    "default_pph23_rate": 2
  }
}
```

### `PUT /settings/company`
Update konfigurasi (hanya role `owner`/`admin`). Berguna kalau nanti tarif PPN berubah lagi – cukup update di sini, semua invoice baru otomatis pakai nilai terbaru.

---

## 1. Modul Customer

### `POST /customers`
Membuat customer baru. Field `customer_type` **penting** untuk perhitungan PPh 23 nanti – hanya customer `badan_usaha` yang punya kewajiban memotong PPh 23, customer `perorangan` umumnya tidak.

Request body:
```json
{
  "name": "PT Sumber Makmur",
  "customer_type": "badan_usaha",
  "phone": "081234567890",
  "email": "contact@sumbermakmur.com",
  "address": "Jl. Industri No. 12, Surabaya",
  "company_name": "PT Sumber Makmur"
}
```

Response `201`: object customer lengkap dengan `id`, `created_at`.

### `GET /customers`
List customer dengan pagination & pencarian.
Query params: `page`, `limit`, `search`, `customer_type`

### `GET /customers/{id}`
Detail customer + nested `machines`.

### `PUT /customers/{id}` / `DELETE /customers/{id}`
Update (partial) / soft delete seperti sebelumnya.

### Sub-resource: Machines
`POST /customers/{customer_id}/machines`, `GET /customers/{customer_id}/machines` – tidak berubah dari versi sebelumnya.

---

## 2. Modul Job (Work Order)

Tidak banyak berubah dari kontrak sebelumnya. Endpoint utama: `POST /jobs`, `GET /jobs`, `GET /jobs/{id}`, `PATCH /jobs/{id}/status`, `PATCH /jobs/{id}/assign`.

### `POST /jobs/{id}/costs` – **diperbarui**
Menambahkan komponen biaya. Sekarang ada `purchase_price` (harga beli, khusus `spare_part`) dan `selling_price` (harga jual, masuk ke invoice).

Request body:
```json
{
  "cost_type": "spare_part",
  "description": "Bearing spindle SKF 6205",
  "quantity": 2,
  "purchase_price": 120000,
  "selling_price": 150000
}
```
Untuk `cost_type = labor` atau `transport`, `purchase_price` tidak diisi (null) – cukup `selling_price` (tidak ada "harga beli" untuk jasa).

Response: `subtotal` dihitung otomatis di backend = `selling_price × quantity`, tetap tidak menerima input `subtotal` dari client (validasi integritas data seperti sebelumnya).

### `GET /jobs/{id}/costs`
List komponen biaya + `meta` tambahan: `total_selling` (untuk subtotal invoice) dan `total_margin` (`SUM((selling_price - purchase_price) × quantity)` untuk item spare part – berguna untuk dashboard margin per job nanti).

### `DELETE /jobs/{id}/costs/{cost_id}`
Tidak berubah.

---

## 3. Modul Costing / Invoice – **paling banyak berubah**

### `POST /jobs/{id}/invoice`
Generate invoice dari job `completed`. Backend mengambil `default_tax_percentage` dan `default_pph23_rate` dari `company_settings` (bisa di-override per invoice kalau perlu kasus khusus).

Request body:
```json
{
  "tax_percentage": 11,
  "due_date": "2026-08-15"
}
```

**Logika perhitungan di backend:**
1. `subtotal` = SUM(`job_costs.subtotal`) – semua cost_type
2. `tax_amount` = `subtotal × tax_percentage` (PPN – karena PT sudah PKP, ini **selalu dihitung**, tidak bisa di-skip)
3. `total` = `subtotal + tax_amount` – ini nilai yang tertulis di invoice/faktur, yang ditagih ke customer
4. `dpp_pph23` = SUM(`job_costs.subtotal` **WHERE cost_type IN ('labor', 'transport')**) – exclude `spare_part`, sesuai aturan PPh 23 yang basisnya cuma komponen jasa
5. `pph23_estimated_amount` = `dpp_pph23 × default_pph23_rate` – **hanya dihitung kalau `customer.customer_type = 'badan_usaha'`**, kalau customer perorangan nilainya 0
6. `expected_receivable` = `total - pph23_estimated_amount` – estimasi uang riil yang akan diterima setelah customer potong PPh 23

Response `201`:
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "invoice_number": "INV-2026-0042",
    "nomor_faktur_pajak": null,
    "job_id": "uuid",
    "subtotal": 1500000,
    "tax_percentage": 11,
    "tax_amount": 165000,
    "total": 1665000,
    "dpp_pph23": 800000,
    "pph23_estimated_amount": 16000,
    "expected_receivable": 1649000,
    "status": "draft",
    "due_date": "2026-08-15",
    "created_at": "2026-07-28T10:00:00Z"
  }
}
```

### `PATCH /invoices/{id}/faktur-pajak`
Endpoint baru – untuk mengisi `nomor_faktur_pajak` setelah faktur pajak dibuat secara manual di sistem e-Faktur DJP (nomor faktur **tidak digenerate oleh aplikasi kita**, karena itu domain resmi DJP/Coretax – aplikasi cuma menyimpan referensinya untuk dokumentasi).

Request body:
```json
{ "nomor_faktur_pajak": "010.001-26.00000123" }
```

### `GET /invoices`, `GET /invoices/{id}`, `PATCH /invoices/{id}/status`
Tidak berubah secara struktur, hanya response-nya sekarang membawa field-field baru di atas.

### `POST /invoices/{id}/payments` – **diperbarui**
Menambahkan field opsional `bukti_potong_pph23_ref` untuk mencatat referensi dokumen bukti potong PPh 23 dari customer (kalau ada) – supaya waktu rekonsiliasi, jelas kenapa `amount` yang diterima lebih kecil dari `total` invoice (bukan customer nunggak, tapi memang dipotong pajak).

Request body:
```json
{
  "amount": 1649000,
  "payment_method": "transfer",
  "bukti_potong_pph23_ref": "BP-23/SM/2026/0042",
  "notes": "Transfer BCA, sudah dipotong PPh 23 sesuai bukti potong terlampir"
}
```
Logika pelunasan otomatis diubah: invoice dianggap `paid` kalau `SUM(payments.amount) + SUM(pph23 yang tercatat via bukti potong) >= total`, bukan cuma `SUM(payments.amount) >= total` seperti versi sebelumnya – supaya invoice yang dipotong pajak tidak selamanya nyangkut di status "belum lunas" padahal customer sudah bayar penuh (secara riil).

### `GET /invoices/{id}/payments`
Tidak berubah.

---

## Catatan Desain (update dari versi sebelumnya)

1. **Kenapa `purchase_price` dan `selling_price` dipisah, bukan cuma satu `unit_price`?** Supaya margin per spare part bisa dihitung (`selling_price - purchase_price`), dan ini jadi dasar metrik dashboard "margin per job" nanti – bukan cuma omzet, tapi profitabilitas riil.
2. **Kenapa `dpp_pph23` exclude spare_part?** Karena secara aturan, dasar potong PPh 23 untuk jasa perbaikan mesin cuma dari komponen jasanya, bukan dari penggantian barang/material – kalau tidak dipisah di level data, perusahaan bisa dipotong pajak lebih besar dari seharusnya.
3. **Kenapa `pph23_estimated_amount` bukan pengurang langsung di `total`?** Karena `total` adalah nilai resmi di invoice/faktur pajak (dokumen legal), sedangkan pemotongan PPh 23 adalah urusan antara customer dan negara – aplikasi cuma perlu tahu estimasinya untuk rekonsiliasi kas, bukan mengubah nilai invoice itu sendiri.
4. **Kenapa `nomor_faktur_pajak` nullable dan endpoint terpisah?** Karena proses pembuatan faktur pajak elektronik di DJP terjadi di luar sistem kita – job aplikasi cuma menyimpan hasilnya sebagai referensi/dokumentasi, bukan menggenerate nomor faktur sendiri (itu wewenang DJP).
