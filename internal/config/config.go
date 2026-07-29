package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	AppPort     string
	DatabaseURL string
	RedisURL    string
	JWTSecret   string
	SMTPHost    string
	SMTPPort    string
	SMTPUser    string
	SMTPPass    string
	SMTPFrom    string
}

// Load membaca konfigurasi dari file .env (kalau ada) lalu dari environment variable.
// Environment variable asli (misal dari docker-compose atau VPS) selalu menang
// dibanding isi file .env, supaya gampang override saat deploy.
func Load() Config {
	_ = godotenv.Load()

	return Config{
		AppPort:     getEnv("APP_PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", ""),
		RedisURL:    getEnv("REDIS_URL", "localhost:6379"),
		// JWTSecret sengaja TIDAK punya fallback aman seperti field lain -
		// secret dipakai menandatangani token akses; kalau nilainya ketebak
		// (mis. default kosong atau string tetap di kode), siapapun bisa
		// memalsukan token milik user manapun. Lebih baik server gagal
		// start dengan pesan jelas daripada jalan dengan secret yang lemah.
		JWTSecret: mustGetEnv("JWT_SECRET"),
		// SMTP* SEBALIKNYA boleh kosong (tidak pakai mustGetEnv) - beda dari
		// JWTSecret karena notifikasi adalah fitur pendukung, bukan sesuatu
		// yang membuat SELURUH aplikasi tidak aman kalau belum di-set.
		// Kalau kosong, main.go cuma mencatat peringatan saat start; setiap
		// percobaan kirim email akan gagal dan tercatat di tabel
		// notifications, tapi server tetap boleh jalan untuk fitur lain.
		SMTPHost: getEnv("SMTP_HOST", ""),
		SMTPPort: getEnv("SMTP_PORT", "587"),
		SMTPUser: getEnv("SMTP_USER", ""),
		SMTPPass: getEnv("SMTP_PASS", ""),
		SMTPFrom: getEnv("SMTP_FROM", ""),
	}
}

func mustGetEnv(key string) string {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		log.Fatalf("%s wajib diisi (lihat .env.example)", key)
	}
	return v
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
