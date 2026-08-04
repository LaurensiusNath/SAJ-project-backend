# CNC Service Project Management App — Backend Context

## Konteks Proyek

Aplikasi project management untuk perusahaan servis mesin CNC (PT, sudah PKP).
Tujuan utama: **bukan sekadar bikin aplikasi jadi, tapi belajar software engineering
tingkat lanjut** — mulai dari development, testing, version control yang benar,
CI/CD, sampai deployment di VPS. Proyek ini dipakai riil oleh perusahaan (bapak
pemilik proyek), tapi prioritas kedua setelah pembelajaran.

Masalah bisnis yang diselesaikan aplikasi ini: pencatatan keuangan yang sering
"loss", jadwal servis yang gampang lupa, notifikasi (email/WhatsApp), perhitungan
biaya jasa (termasuk pajak PPN/PPh 23, spare part dengan markup, biaya transport),
dan dashboard visualisasi.

## Referensi Dokumen (baca dulu sebelum kerja di modul terkait)

- `docs/erd.md` — Entity Relationship Diagram lengkap (Mermaid) — **v3, hasil sinkronisasi audit 2026-07-30**
- `docs/api-contract.md` — API contract v3, termasuk modul Auth/User & Notification yang sebelumnya tidak terdokumentasi

> **PENTING, lesson learned dari audit 2026-07-30**: sebelumnya `docs/erd.md` ternyata
> **tidak pernah benar-benar jadi file di repo** — cuma pernah dibagikan sebagai gambar di
> chat awal, sementara CLAUDE.md ini sudah lama mereferensikannya seolah-olah ada. Akibatnya
> implementasi berjalan tanpa acuan skema yang benar-benar bisa dibaca, dan drift antara
> desain-vs-kode baru ketahuan belakangan lewat audit manual, bukan lewat kontrol berkelanjutan.
>
> **Aturan mulai sekarang**: kalau kamu (Claude Code) membuat tabel baru, mengubah enum/constraint,
> menambah endpoint, atau mengambil keputusan desain yang menyimpang dari `docs/erd.md` /
> `docs/api-contract.md` — **update kedua file itu di PR/commit yang sama**, jangan tunda sampai
> ada audit terpisah. Kalau perubahannya signifikan secara desain (bukan detail implementasi kecil),
> sebutkan eksplisit di commit message atau PR description supaya gampang ditelusuri nanti.
- Aturan pajak (ringkasan, detail ada di api-contract.md):
  - PT **sudah PKP** — PPN 11% selalu dipungut di tiap invoice
  - PPh 23 (2%) dipotong oleh **customer**, bukan oleh kita, dan **hanya dari komponen jasa
    (labor + transport), bukan dari spare part** — makanya `job_costs` dipisah per `cost_type`
  - Spare part dijual dengan **markup** — butuh `purchase_price` dan `selling_price` terpisah

## Tech Stack (Backend)

- Go 1.25 (naik dari 1.22 setelah testcontainers-go ditambahkan untuk
  integration test - dependency graph-nya mewajibkan Go >= 1.25, lihat
  commit terkait), framework HTTP: **Gin**
- Database access: **sqlc** (raw SQL, type-safe) — sengaja dipilih daripada ORM
  penuh (GORM) supaya query optimization terasa langsung
- PostgreSQL (data utama) + Redis (cache, queue, session)
- Migration: `golang-migrate`
- Testing: `testify` (unit), `testcontainers-go` (integration)

## Status Saat Ini

Skeleton awal sudah dibuat manual (bukan oleh Claude Code): `go.mod`,
`docker-compose.yml`, `Dockerfile`, migration `000001`-`000003` (users, customers,
machines), `sqlc.yaml`, query pertama (`db/queries/customers.sql`), dan
`cmd/api/main.go` (Gin + koneksi pgxpool + health check). **Belum ada
implementasi layer domain/service/handler.**

Urutan implementasi modul yang direncanakan: **Customer → Job → Costing/Invoice**
(mengikuti urutan dependency). Customer jadi modul percontohan — pola yang
dibangun di sini akan ditiru untuk modul berikutnya.

---

## PENTING — Cara Kerja yang Saya Inginkan dari Claude Code

Saya belajar software engineering lewat proyek ini, jadi **jangan cuma
eksekusi task dan selesai**. Ikuti prinsip ini di setiap sesi:

### 1. Jelaskan sebelum eksekusi
Sebelum implementasi fitur/layer baru, jelaskan singkat **pendekatan dan
alasannya** (bukan cuma "saya akan buat file X"), terutama kalau ada
trade-off atau lebih dari satu cara valid. Kalau saya minta sesuatu yang
menyimpang dari Clean Architecture yang sudah disepakati, tanya dulu atau
jelaskan konsekuensinya sebelum jalan.

### 2. Version control sebagai bahan belajar, bukan formalitas
- Commit kecil dan sering, bukan satu commit raksasa per fitur
- Pakai **Conventional Commits** (`feat:`, `fix:`, `refactor:`, `test:`, `chore:`, `docs:`)
  dan jelaskan kenapa suatu perubahan masuk kategori tertentu
- Kalau ada momen yang cocok untuk branching strategy (misal mulai fitur baru),
  jelaskan alasannya (`feature/customer-module` dst) — jangan asumsikan saya
  sudah paham workflow git branching, jelaskan waktu pertama kali dipakai
- Kalau relevan, jelaskan kapan sebaiknya rebase vs merge

### 3. Testing dibangun bareng, bukan ditambahkan belakangan
- Setiap modul baru (usecase/service) harus disertai unit test saat itu juga,
  bukan "nanti di fase testing terpisah"
- Waktu pertama kali pakai `testcontainers-go`, jelaskan kenapa ini lebih baik
  daripada mock database untuk integration test
- Kalau ada N+1 query atau query yang berpotensi lambat, tunjukkan cara
  cek pakai `EXPLAIN ANALYZE` dan jelaskan cara bacanya — jangan cuma
  langsung optimasi diam-diam

> **Catatan environment, ditemukan saat modul Dashboard (PR #16)**: di
> environment development ini, Go toolchain cuma ter-install di Windows
> native, sementara Docker cuma ter-install di dalam WSL2 (bukan Docker
> Desktop dengan integrasi Windows — tidak ada port-forwarding otomatis
> dari WSL2 ke `localhost` Windows). Akibatnya `go test -tags=integration
> ./...` (testcontainers-go) **gagal total kalau dijalankan dari shell
> Windows native** — errornya `open //./pipe/docker_engine: The system
> cannot find the file specified` — karena testcontainers-go butuh bicara
> langsung ke Docker Engine lewat socket, bukan lewat port TCP yang
> dipetakan (beda dari sekadar `docker compose up` yang portnya bisa
> diakses lintas WSL2↔Windows via IP WSL2, lihat catatan verifikasi
> live). Solusi yang terbukti jalan: install Go portable di **dalam**
> WSL2 (extract tarball ke `$HOME`, tanpa perlu root/apt, `GOPATH`/
> `GOCACHE` diarahkan ke folder terpisah supaya tidak bentrok dengan
> `GOROOT`), lalu jalankan `go test -tags=integration ./...` dari sana —
> bukan cuma `docker` command yang perlu di-routing lewat WSL2, seluruh
> proses `go test`-nya juga harus.

### 4. Arsitektur — modular monolith dulu, microservice belakangan
- Ikuti struktur Clean Architecture yang sudah ada: `internal/<domain>/domain.go`,
  `repository.go`, `service.go`, `handler.go`
- Modul harus tetap loosely-coupled satu sama lain (komunikasi lewat interface,
  bukan saling import langsung antar domain) — ini persiapan supaya nanti bisa
  di-extract jadi microservice terpisah (rencana: Notification Service jadi
  yang pertama dicoba diekstrak, karena paling independen)
- Kalau saya minta shortcut yang bikin modul jadi tightly-coupled, ingatkan saya

### 5. CI/CD — bangun bertahap, jangan langsung kompleks
- Mulai dari pipeline sederhana (lint → test → build), baru nanti ditambah
  build image & deploy
- Setiap kali menambah step baru ke GitHub Actions, jelaskan apa yang
  step itu cegah/pastikan (misal: kenapa lint jalan sebelum test, kenapa
  test harus pass sebelum build image)

### 6. Kalau saya salah paham konsep, koreksi
Saya lebih suka dikoreksi dengan penjelasan daripada dibiarkan lanjut dengan
pemahaman yang salah — termasuk soal Go idiomatic patterns, SQL, maupun
konsep arsitektur.

### 7. ACID & Concurrency — ini tujuan belajar eksplisit, jangan dilewatkan
Saya ingin paham database transaction bukan cuma teori, tapi lewat kasus nyata
di app ini. **Titik-titik berikut WAJIB dibungkus transaction, dan WAJIB
dijelaskan kenapa saat pertama kali diimplementasikan:**

- **`POST /jobs/{id}/invoice`** — baca `job_costs`, hitung subtotal/pajak,
  insert `invoices`, update status `jobs`. Ini contoh Atomicity: semua-atau-tidak-sama-sekali.
- **`POST /invoices/{id}/payments`** — insert payment lalu cek apakah total
  pembayaran sudah cukup untuk update status invoice jadi `paid`. Ini rawan
  **lost update** kalau dua payment masuk bersamaan tanpa row lock. Pakai
  `SELECT ... FOR UPDATE` pada row invoice sebelum baca `SUM(payments)`.
- **`PATCH /jobs/{id}/assign`** — dua admin bisa saja assign teknisi berbeda
  ke job yang sama hampir bersamaan. Diskusikan pilihan **pessimistic locking**
  (`SELECT FOR UPDATE`) vs **optimistic locking** (kolom `updated_at`/`version`
  dicek di `WHERE` clause update) untuk kasus ini, dan jelaskan kenapa pilih
  salah satu.

Hal spesifik yang perlu dijelaskan setiap kali relevan (jangan cuma diterapkan
diam-diam di kode):
- **Isolation level**: default Postgres itu Read Committed — jelaskan kapan itu
  cukup, dan kapan perlu naik ke Repeatable Read/Serializable (misal nanti di
  modul Dashboard yang butuh snapshot konsisten dari banyak tabel)
- **Lock timeout vs deadlock**: jelaskan beda antara "menunggu lock tanpa batas
  waktu" (perlu `SET lock_timeout`) vs deadlock (dua transaction saling
  menunggu — Postgres auto-detect dan abort salah satunya dengan error `40P01`,
  tapi kode Go tetap perlu retry logic untuk handle error itu)
- **Optimistic vs pessimistic locking**: trade-off-nya (pessimistic lebih aman
  tapi bisa bikin transaction lain menunggu/blocking; optimistic lebih murah
  tapi butuh retry di sisi aplikasi kalau conflict)

Kalau memungkinkan, buatkan juga **test yang sengaja mensimulasikan race
condition** (dua goroutine menembak endpoint yang sama secara concurrent) untuk
membuktikan masalahnya ada dulu sebelum diperbaiki dengan locking — supaya saya
lihat langsung "lost update" itu bukan cuma teori di atas kertas.

> **Klarifikasi dari audit 2026-07-30**: locking-nya sendiri sudah benar
> (pessimistic di payment, optimistic di assign), TAPI pembuktiannya cuma lewat
> script manual yang dijalankan sekali lalu **dihapus dari repo**. Itu tidak
> cukup — kalau nanti ada refactor dan bug lost-update muncul lagi, tidak ada
> yang akan gagal di CI untuk memberi tahu. Race-condition test **wajib jadi
> automated test yang di-commit dan jalan di CI** (pakai `testcontainers-go`
> dengan Postgres asli, bukan mock), bukan skrip ad-hoc sekali pakai.

---

## Roadmap Fase (untuk konteks urutan kerja)

1. ~~Design & Planning~~ (ERD, API contract — sudah selesai, lihat `docs/`)
2. **Backend Modular Monolith** — tahap sekarang, mulai dari modul Customer
3. Frontend Next.js (belum mulai, di repo terpisah)
4. Fitur spesifik domain (costing engine, scheduling, notification)
5. Dockerize (base sudah ada, perlu disempurnakan)
6. Testing menyeluruh (integration + e2e)
7. CI/CD (GitHub Actions — deploy VPS)
8. Advanced: extract microservice, query optimization eksperimen, monitoring
   (Prometheus/Grafana), caching dengan Redis

## Konvensi Kode

- Error handling: selalu wrap error dengan context (`fmt.Errorf("...: %w", err)`),
  jangan silent-fail
- Response format API selalu ikuti kontrak di `docs/api-contract.md`
  (`{ "success": bool, "data"/"error": ... }`)
- Nilai finansial (subtotal, tax, dll) **selalu dihitung di backend**, tidak
  pernah menerima input langsung dari client — ini prinsip keamanan data yang
  sudah disepakati di desain
