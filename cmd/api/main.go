package main

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nathan/cnc-pm-backend/internal/config"
	"github.com/nathan/cnc-pm-backend/internal/customer"
	"github.com/nathan/cnc-pm-backend/internal/job"
	"github.com/nathan/cnc-pm-backend/internal/repository/sqlcgen"
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

	queries := sqlcgen.New(dbPool)

	customerRepo := customer.NewRepository(queries)
	customerService := customer.NewService(customerRepo)
	customerHandler := customer.NewHandler(customerService)
	customerHandler.RegisterRoutes(router.Group("/api/v1"))

	jobRepo := job.NewRepository(queries)
	jobService := job.NewService(jobRepo)
	jobHandler := job.NewHandler(jobService)
	jobHandler.RegisterRoutes(router.Group("/api/v1"))

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

	log.Printf("server berjalan di port %s", cfg.AppPort)
	if err := router.Run(":" + cfg.AppPort); err != nil {
		log.Fatalf("server gagal jalan: %v", err)
	}
}
