# sqlc diambil dari image resminya (binary sudah dikompilasi), BUKAN lewat
# `go install` di image golang:1.22-alpine - sqlc v1.31.1 sendiri butuh
# Go >= 1.26 untuk DIKOMPILASI, padahal kita cuma butuh MENJALANKANNYA.
# Memaksa `go install` di golang:1.22-alpine gagal (dicoba dulu, terbukti
# error "requires go >= 1.26.0"). Mengambil binary jadi dari image resmi
# menghindari isu ini sepenuhnya - versi Go yang mengompilasi sqlc tidak
# perlu sama dengan versi Go yang mengompilasi aplikasi kita sendiri.
FROM sqlc/sqlc:1.31.1 AS sqlc

# ---- build stage ----
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY --from=sqlc /workspace/sqlc /usr/local/bin/sqlc

COPY go.mod go.sum* ./
RUN go mod download
COPY . .

# internal/repository/sqlcgen/ sengaja di-gitignore (generated code, lihat
# .gitignore) - checkout bersih seperti build context Docker ini TIDAK
# PUNYA folder itu sama sekali. Tanpa step ini, `go build` di bawah gagal
# karena cmd/api mengimpor paket yang tidak ada. CI (.github/workflows/ci.yml)
# sudah menjalankan step yang sama sebelum test/build - Dockerfile ini
# sebelumnya luput menyalin pola itu (ditemukan lewat audit 2026-07-30).
RUN sqlc generate

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api

# ---- run stage ----
FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/api .
# TIDAK menyalin .env.example ke .env di sini (beda dari sebelumnya) -
# membundel file env statis ke dalam image adalah praktik yang salah:
# nilai sungguhan (JWT_SECRET, kredensial DB, dst) harus selalu disuntikkan
# saat container dijalankan (docker-compose env_file/environment, atau
# secret manager di VPS), bukan ikut ter-bake ke image. docker-compose.yml
# sudah menyediakan env_file+environment untuk service ini.
EXPOSE 8080
CMD ["./api"]
