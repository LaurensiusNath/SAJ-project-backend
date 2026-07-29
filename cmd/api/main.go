package main

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/nathan/cnc-pm-backend/internal/auth"
	"github.com/nathan/cnc-pm-backend/internal/config"
	"github.com/nathan/cnc-pm-backend/internal/customer"
	"github.com/nathan/cnc-pm-backend/internal/invoice"
	"github.com/nathan/cnc-pm-backend/internal/job"
	"github.com/nathan/cnc-pm-backend/internal/repository/sqlcgen"
	"github.com/nathan/cnc-pm-backend/internal/settings"
	"github.com/nathan/cnc-pm-backend/internal/user"
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

	// authService dibuat lebih dulu - dipakai baik untuk mendaftarkan
	// POST /auth/login (publik) maupun untuk middleware RequireAuth yang
	// dipasang di semua route group lain.
	userRepo := user.NewRepository(queries)
	userService := user.NewService(userRepo)
	authService := auth.NewService(userRepo, cfg.JWTSecret)
	authHandler := auth.NewHandler(authService)

	// publicGroup: SATU-SATUNYA route tanpa RequireAuth, sesuai
	// api-contract.md ("semua endpoint kecuali /auth/login butuh Bearer token").
	publicGroup := router.Group("/api/v1")
	authHandler.RegisterRoutes(publicGroup)

	// protectedGroup: semua endpoint bisnis, wajib Bearer token valid.
	protectedGroup := router.Group("/api/v1")
	protectedGroup.Use(authService.RequireAuth())

	// adminOnly dipakai untuk route yang butuh RequireAuth DAN role
	// owner/admin sekaligus (POST /users). Untuk endpoint yang cuma
	// SEBAGIAN method-nya dibatasi role (PUT /settings/company, tapi GET-nya
	// tetap untuk semua user login), requireAdmin di-inject langsung ke
	// handler yang bersangkutan (lihat settings.Handler.RegisterRoutes),
	// bukan lewat route group terpisah.
	requireAdmin := auth.RequireRole(user.RoleOwner, user.RoleAdmin)
	adminGroup := router.Group("/api/v1")
	adminGroup.Use(authService.RequireAuth(), requireAdmin)

	userHandler := user.NewHandler(userService)
	userHandler.RegisterRoutes(adminGroup)

	customerRepo := customer.NewRepository(queries)
	customerService := customer.NewService(customerRepo)
	machineRepo := customer.NewMachineRepository(queries)
	machineService := customer.NewMachineService(machineRepo, customerRepo)
	customerHandler := customer.NewHandler(customerService, machineService)
	customerHandler.RegisterRoutes(protectedGroup)
	machineHandler := customer.NewMachineHandler(machineService)
	machineHandler.RegisterRoutes(protectedGroup)

	jobRepo := job.NewRepository(queries)
	jobService := job.NewService(jobRepo)
	jobHandler := job.NewHandler(jobService)
	jobHandler.RegisterRoutes(protectedGroup)

	jobCostRepo := job.NewCostRepository(queries)
	jobCostService := job.NewCostService(jobCostRepo, jobRepo)
	jobCostHandler := job.NewCostHandler(jobCostService)
	jobCostHandler.RegisterRoutes(protectedGroup)

	settingsRepo := settings.NewRepository(queries)
	settingsService := settings.NewService(settingsRepo)
	settingsHandler := settings.NewHandler(settingsService)
	settingsHandler.RegisterRoutes(protectedGroup, requireAdmin)

	// invoice.NewRepository butuh dbPool (bukan cuma queries) - CreateFromJob
	// dan RecordPayment membuka transaksi sendiri (lihat internal/invoice/repository.go).
	invoiceRepo := invoice.NewRepository(dbPool, queries)
	invoiceService := invoice.NewService(invoiceRepo)
	invoiceHandler := invoice.NewHandler(invoiceService)
	invoiceHandler.RegisterRoutes(protectedGroup)

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
