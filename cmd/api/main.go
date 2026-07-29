package main

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/nathan/cnc-pm-backend/internal/config"
	"github.com/nathan/cnc-pm-backend/internal/customer"
	"github.com/nathan/cnc-pm-backend/internal/invoice"
	"github.com/nathan/cnc-pm-backend/internal/job"
	"github.com/nathan/cnc-pm-backend/internal/repository/sqlcgen"
	"github.com/nathan/cnc-pm-backend/internal/settings"
)

func init() {
	// docs/api-contract.md menunjukkan field uang sebagai angka JSON polos
	// (mis. "subtotal": 1500000), bukan string. Keputusan sadar mengikuti
	// format itu persis - default aman decimal.Decimal sebenarnya string
	// berkutip (menghindari frontend parse jadi float64 dan kehilangan
	// presisi), tapi trade-off ini sudah didiskusikan dan dipilih sesuai
	// kontrak yang ada. Frontend WAJIB pakai library decimal
	// (mis. decimal.js/big.js), bukan Number()/parseFloat(), saat membaca
	// field-field ini.
	decimal.MarshalJSONWithoutQuotes = true
}

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
	machineRepo := customer.NewMachineRepository(queries)
	machineService := customer.NewMachineService(machineRepo, customerRepo)
	customerHandler := customer.NewHandler(customerService, machineService)
	customerHandler.RegisterRoutes(router.Group("/api/v1"))
	machineHandler := customer.NewMachineHandler(machineService)
	machineHandler.RegisterRoutes(router.Group("/api/v1"))

	jobRepo := job.NewRepository(queries)
	jobService := job.NewService(jobRepo)
	jobHandler := job.NewHandler(jobService)
	jobHandler.RegisterRoutes(router.Group("/api/v1"))

	jobCostRepo := job.NewCostRepository(queries)
	jobCostService := job.NewCostService(jobCostRepo, jobRepo)
	jobCostHandler := job.NewCostHandler(jobCostService)
	jobCostHandler.RegisterRoutes(router.Group("/api/v1"))

	settingsRepo := settings.NewRepository(queries)
	settingsService := settings.NewService(settingsRepo)
	settingsHandler := settings.NewHandler(settingsService)
	settingsHandler.RegisterRoutes(router.Group("/api/v1"))

	// invoice.NewRepository butuh dbPool (bukan cuma queries) - CreateFromJob
	// dan RecordPayment membuka transaksi sendiri (lihat internal/invoice/repository.go).
	invoiceRepo := invoice.NewRepository(dbPool, queries)
	invoiceService := invoice.NewService(invoiceRepo)
	invoiceHandler := invoice.NewHandler(invoiceService)
	invoiceHandler.RegisterRoutes(router.Group("/api/v1"))

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
