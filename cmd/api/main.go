package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/nathan/cnc-pm-backend/internal/auth"
	"github.com/nathan/cnc-pm-backend/internal/config"
	"github.com/nathan/cnc-pm-backend/internal/customer"
	"github.com/nathan/cnc-pm-backend/internal/dashboard"
	"github.com/nathan/cnc-pm-backend/internal/invoice"
	"github.com/nathan/cnc-pm-backend/internal/job"
	"github.com/nathan/cnc-pm-backend/internal/notification"
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

	if cfg.SMTPHost == "" {
		log.Println("PERINGATAN: SMTP_HOST kosong - semua notifikasi email akan gagal terkirim (tetap tercatat di tabel notifications)")
	}
	notificationRepo := notification.NewRepository(queries)
	mailer := notification.NewSMTPMailer(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFrom)
	notifier := notification.NewService(notificationRepo, mailer)

	// authService dibuat lebih dulu - dipakai baik untuk mendaftarkan
	// POST /auth/login (publik) maupun untuk middleware RequireAuth yang
	// dipasang di semua route group lain.
	userRepo := user.NewRepository(queries)
	userService := user.NewService(userRepo)
	authService := auth.NewService(userRepo, cfg.JWTSecret)
	// secureCookie=true HANYA kalau APP_ENV=production - development lewat
	// http://localhost harus tetap bisa login (browser menolak cookie Secure
	// di koneksi non-HTTPS). Lihat auth.Handler.setAccessTokenCookie dan
	// docs/api-contract.md#0 (Catatan arsitektur 2026-07-31).
	secureCookie := cfg.AppEnv == "production"
	authHandler := auth.NewHandler(authService, secureCookie)

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

	// jobCostRepo dibuat lebih dulu - jobService butuh CostRepository juga
	// sekarang (buat menyusun field `costs` di GET /jobs/{id}, lihat
	// job.Service.GetDetail).
	jobCostRepo := job.NewCostRepository(queries)

	// job.NewRepository butuh dbPool juga sekarang (sebelumnya cukup
	// Queries) - UpdateStatus membungkus UPDATE jobs + INSERT
	// job_status_history dalam satu transaksi (lihat internal/job/repository.go).
	jobRepo := job.NewRepository(dbPool, queries)
	jobService := job.NewService(jobRepo, customerRepo, userRepo, jobCostRepo, notifier)
	jobHandler := job.NewHandler(jobService)
	jobHandler.RegisterRoutes(protectedGroup)

	reminderService := job.NewReminderService(jobRepo, customerRepo, notifier, notificationRepo)

	jobCostService := job.NewCostService(jobCostRepo, jobRepo)
	jobCostHandler := job.NewCostHandler(jobCostService)
	jobCostHandler.RegisterRoutes(protectedGroup)

	settingsRepo := settings.NewRepository(queries)
	settingsService := settings.NewService(settingsRepo)
	settingsHandler := settings.NewHandler(settingsService)
	settingsHandler.RegisterRoutes(protectedGroup, requireAdmin)

	// invoice.NewRepository butuh dbPool (bukan cuma queries) - CreateFromJob
	// dan RecordPayment membuka transaksi sendiri (lihat internal/invoice/repository.go).
	invoiceRepo := invoice.NewRepository(dbPool, queries, notifier)
	invoiceService := invoice.NewService(invoiceRepo)
	invoiceHandler := invoice.NewHandler(invoiceService)
	invoiceHandler.RegisterRoutes(protectedGroup)

	// dashboard.NewRepository butuh dbPool (bukan cuma queries) -
	// GetSummary membuka transaksi REPEATABLE READ read-only sendiri (lihat
	// internal/dashboard/repository.go). Route-nya dipasang di adminGroup
	// yang SUDAH ada (dipakai user.Handler) - owner/admin saja, sama
	// persis kebutuhan kontrak (403 untuk teknisi), tanpa perlu grup baru.
	dashboardRepo := dashboard.NewRepository(dbPool, queries)
	dashboardService := dashboard.NewService(dashboardRepo)
	dashboardHandler := dashboard.NewHandler(dashboardService)
	dashboardHandler.RegisterRoutes(adminGroup)

	// Reminder scheduled_date DAN auto-transition invoice overdue jalan di
	// goroutine terpisah lewat ticker yang SAMA (bukan dua infrastruktur
	// terjadwal) - lihat runReminderScheduler untuk alasan interval & cek
	// pertama langsung saat startup. invoice.Service.MarkOverdue tidak bisa
	// jadi bagian dari job.ReminderService sendiri karena invoice package
	// sudah mengimpor job (circular import kalau dibalik) - main.go yang
	// menjembatani keduanya di satu ticker.
	go runReminderScheduler(reminderService, invoiceService)

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

// runReminderScheduler menjalankan job.ReminderService.CheckAndNotify DAN
// invoice.Service.MarkOverdue di ticker yang SAMA - satu infrastruktur
// terjadwal dipakai bareng oleh dua urusan yang beda modul (job's
// scheduled_date reminder, invoice's overdue auto-transition), bukan dua
// ticker terpisah. Cek pertama terjadi SEGERA saat startup (bukan menunggu
// satu interval dulu) - baik reminder maupun invoice yang sudah due tidak
// perlu menunggu sampai tick pertama lewat, dan ini juga yang membuat
// fitur ini gampang diverifikasi manual (restart server = cek langsung
// jalan).
//
// Interval 1 jam dipilih karena ExistsSentToday (job) dan kondisi WHERE
// status='sent' (invoice, idempotent by nature) sama-sama tidak butuh
// presisi ke menit - cukup "dalam sejam sejak due boleh sedikit telat".
// time.Ticker standar dipakai, bukan library cron (mis. robfig/cron) -
// cukup untuk kebutuhan project ini, tidak butuh jadwal presisi/multi-job
// yang jadi alasan utama pakai library cron sungguhan.
func runReminderScheduler(jobSvc *job.ReminderService, invoiceSvc *invoice.Service) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		ctx := context.Background()
		if err := jobSvc.CheckAndNotify(ctx); err != nil {
			log.Printf("reminder check gagal: %v", err)
		}
		if overdue, err := invoiceSvc.MarkOverdue(ctx); err != nil {
			log.Printf("mark overdue invoices gagal: %v", err)
		} else if len(overdue) > 0 {
			log.Printf("%d invoice ditandai overdue", len(overdue))
		}
		<-ticker.C
	}
}
