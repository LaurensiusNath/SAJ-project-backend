package main

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nathan/cnc-pm-backend/internal/config"
)

func main() {
	cfg := config.Load()

	// Koneksi ke Postgres pakai connection pool (pgxpool), bukan single connection.
	// Ini penting untuk production: tiap request tidak buka koneksi baru dari nol.
	dbPool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("gagal konek ke database: %v", err)
	}
	defer dbPool.Close()

	if err := dbPool.Ping(context.Background()); err != nil {
		log.Fatalf("database tidak bisa di-ping: %v", err)
	}
	log.Println("berhasil konek ke database")

	router := gin.Default()

	// Health check endpoint - wajib ada untuk deployment (dipakai load balancer /
	// orchestrator buat cek apakah service masih hidup)
	router.GET("/health", func(c *gin.Context) {
		if err := dbPool.Ping(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unhealthy",
				"error":  "database unreachable",
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	// TODO: daftarkan route modul Customer di sini setelah handler-nya dibuat
	// customerHandler.RegisterRoutes(router.Group("/api/v1"))

	log.Printf("server berjalan di port %s", cfg.AppPort)
	if err := router.Run(":" + cfg.AppPort); err != nil {
		log.Fatalf("server gagal jalan: %v", err)
	}
}
