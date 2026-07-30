//go:build integration

// Package testhelper menyediakan Postgres ASLI (lewat testcontainers-go)
// untuk integration test - dipakai modul yang perilakunya cuma bisa
// dibuktikan lewat database sungguhan (row locking, transaksi,
// constraint), bukan direplikasi dengan fake Repository in-memory.
//
// File ini dibatasi build tag "integration" (lihat baris pertama) supaya
// TIDAK ikut jalan waktu `go test ./...` biasa (unit test cepat, tanpa
// Docker) - cuma jalan lewat `go test -tags=integration ./...`, yang
// dijalankan sebagai step terpisah di CI (lihat .github/workflows/ci.yml).
// Ini pola standar Go untuk memisahkan unit test (selalu jalan, cepat)
// dari integration test (perlu Docker, lebih lambat), bukan cara untuk
// menyembunyikannya dari CI - keduanya tetap wajib lolos di setiap PR.
package testhelper

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// NewPostgresPool menyalakan container Postgres 16 sekali pakai, menjalankan
// SEMUA migration di db/migrations (termasuk seed company_settings dan
// owner user), lalu mengembalikan pool yang sudah terhubung. Container
// otomatis dimatikan (t.Cleanup) begitu test selesai - tidak ada state yang
// tersisa antar test, beda dari WSL2 Postgres yang dipakai untuk verifikasi
// manual di sepanjang project ini (itu persisten, ini sekali pakai per test).
func NewPostgresPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	// postgres.Run sudah punya wait strategy default (nunggu log "database
	// system is ready to accept connections" muncul dua kali) - tidak perlu
	// dioverride, cukup untuk kebutuhan test ini.
	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("cnc_pm_test"),
		postgres.WithUsername("cncuser"),
		postgres.WithPassword("cncpassword"),
	)
	require.NoError(t, err, "failed to start postgres testcontainer")
	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	// postgres:16-alpine's entrypoint restarts the server process once
	// after running init scripts (standard upstream Postgres image
	// behavior) - the module's wait strategy already waits for the "ready"
	// log line twice to account for this, but on a fast machine the very
	// first connection attempt right after can still land in the brief
	// window where the TCP listener is being torn down/recreated,
	// surfacing as "connection reset by peer" rather than a clean refuse.
	// A short retry loop here is more robust than trying to out-guess
	// that timing with a fixed sleep.
	require.Eventually(t, func() bool {
		conn, pingErr := pgx.Connect(ctx, connStr)
		if pingErr != nil {
			return false
		}
		defer conn.Close(ctx)
		return conn.Ping(ctx) == nil
	}, 15*time.Second, 200*time.Millisecond, "postgres testcontainer never became reachable")

	m, err := migrate.New("file://"+migrationsDir(), connStr)
	require.NoError(t, err, "failed to init migrate")
	require.NoError(t, m.Up(), "failed to run migrations against testcontainer")

	poolCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(poolCtx, connStr)
	require.NoError(t, err, "failed to open pool against testcontainer")
	t.Cleanup(pool.Close)

	return pool
}

// migrationsDir dihitung relatif terhadap lokasi FILE INI (bukan package
// pemanggil), lewat runtime.Caller - supaya benar dipanggil dari package
// manapun (internal/invoice, internal/job, dst) tanpa peduli seberapa
// dalam package itu di struktur folder.
func migrationsDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "db", "migrations")
}
