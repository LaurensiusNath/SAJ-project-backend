package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	AppPort     string
	DatabaseURL string
	RedisURL    string
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
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
