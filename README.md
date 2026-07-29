# CNC PM Backend

Skeleton awal backend Go untuk CNC Service Project Management App.
Stack: Gin (HTTP framework) + sqlc (type-safe SQL) + PostgreSQL + Redis.

## Struktur Folder

```
cmd/api/          -> entrypoint aplikasi (main.go)
internal/config/  -> loader environment variable
internal/customer/-> modul Customer (domain, service, handler) - langkah berikutnya
db/migrations/    -> migration SQL (golang-migrate format)
db/queries/       -> raw SQL query untuk di-generate sqlc jadi Go code
sqlc.yaml         -> config sqlc
```

## Cara Menjalankan (Local Development)

### 1. Install tools yang dibutuhkan
```bash
# golang-migrate (untuk menjalankan migration)
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# sqlc (untuk generate Go code dari db/queries/*.sql)
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

### 2. Nyalakan database & redis
```bash
cp .env.example .env
docker compose up -d postgres redis
```

Isi `JWT_SECRET` di `.env` (wajib, server menolak start kalau kosong):
```bash
openssl rand -hex 32
```

### 3. Jalankan migration
```bash
migrate -path db/migrations -database "postgres://cncuser:cncpassword@localhost:5432/cnc_pm_db?sslmode=disable" up
```

### 4. Generate kode dari SQL query (sqlc)
```bash
sqlc generate
```
Ini akan membuat folder `internal/repository/sqlcgen/` berisi Go struct & function
type-safe hasil generate dari `db/queries/customers.sql`. Folder ini di-gitignore
karena sifatnya generated code, bukan source of truth.

### 5. Download dependency & jalankan server
```bash
go mod tidy
go run ./cmd/api
```

Server jalan di `http://localhost:8080`. Cek `GET /health` untuk memastikan
koneksi database berhasil.

### Alternatif: jalankan semua via Docker Compose
```bash
docker compose up --build
```

## Langkah Selanjutnya

Skeleton ini baru mencakup: koneksi database, health check, dan query SQL untuk
modul Customer (belum ada handler/service-nya). Langkah berikutnya adalah
melengkapi `internal/customer/` dengan layer domain -> repository -> service -> handler
mengikuti pola Clean Architecture yang sudah didiskusikan.
