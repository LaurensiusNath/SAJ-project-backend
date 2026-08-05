package job

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/nathan/cnc-pm-backend/internal/customer"
	"github.com/nathan/cnc-pm-backend/internal/notification"
)

// ReminderService TERPISAH dari Service - dipanggil periodik lewat ticker
// (lihat main.go), bukan dipicu HTTP request seperti method Service
// lainnya. Dipisah supaya Service tetap murni "business logic yang dipicu
// request", sementara ini murni "tugas terjadwal", walau keduanya sama-sama
// berurusan dengan Job + notifikasi.
type ReminderService struct {
	repo             Repository
	customerRepo     customer.Repository
	notifier         notification.Sender
	notificationRepo notification.Repository
}

func NewReminderService(repo Repository, customerRepo customer.Repository, notifier notification.Sender, notificationRepo notification.Repository) *ReminderService {
	return &ReminderService{repo: repo, customerRepo: customerRepo, notifier: notifier, notificationRepo: notificationRepo}
}

// CheckAndNotify mengecek semua job yang butuh reminder (lihat
// ListNeedingReminderCheck) dan mengirim email SEKALI per job per jenis
// reminder per hari (dijaga ExistsSentToday) - aman dipanggil berkali-kali
// (mis. tiap jam lewat ticker di main.go), tidak akan nge-spam job yang
// sama tiap kali ticker jalan.
func (s *ReminderService) CheckAndNotify(ctx context.Context) error {
	jobs, err := s.repo.ListNeedingReminderCheck(ctx)
	if err != nil {
		return fmt.Errorf("list jobs needing reminder check: %w", err)
	}

	today := time.Now()
	for _, j := range jobs {
		subject, body := reminderContent(j, today)
		if subject == "" {
			continue
		}

		already, err := s.notificationRepo.ExistsSentToday(ctx, j.ID, subject)
		if err != nil {
			log.Printf("cek idempotency reminder job %s gagal: %v", j.JobCode, err)
			continue
		}
		if already {
			continue
		}

		cust, err := s.customerRepo.GetByID(ctx, j.CustomerID)
		if err != nil || cust.Email == nil {
			continue
		}

		if err := s.notifier.SendEmail(ctx, notification.EmailInput{
			To: *cust.Email, Subject: subject, Body: body, JobID: &j.ID,
		}); err != nil {
			log.Printf("kirim reminder job %s ke %s gagal: %v", j.JobCode, *cust.Email, err)
		}
	}
	return nil
}

// reminderContent mengklasifikasikan job jadi salah satu dari tiga jenis
// reminder (lewat jadwal/hari ini/besok) berdasarkan scheduled_date
// dibanding "today" - fungsi murni, diuji sendiri tanpa database (lihat
// reminder_test.go). Subject kosong berarti tidak perlu reminder (di luar
// jendela lewat/hari ini/besok - seharusnya sudah tersaring oleh
// ListNeedingReminderCheck, tapi dijaga lagi di sini supaya fungsi ini
// tetap benar walau dipanggil sendiri di luar konteks itu).
func reminderContent(j Job, today time.Time) (subject, body string) {
	if j.ScheduledDate == nil {
		return "", ""
	}
	scheduled := truncateToDate(j.ScheduledDate.Time)
	todayDate := truncateToDate(today)
	tomorrow := todayDate.AddDate(0, 0, 1)

	switch {
	case scheduled.Before(todayDate):
		return "Pengingat: Jadwal Servis Anda Telah Lewat",
			fmt.Sprintf("Jadwal servis untuk job %s (%s) sudah lewat (%s) tapi belum selesai. Mohon segera dijadwalkan ulang.",
				j.JobCode, j.Title, scheduled.Format("2 January 2006"))
	case scheduled.Equal(todayDate):
		return "Pengingat: Jadwal Servis Anda Hari Ini",
			fmt.Sprintf("Jadwal servis untuk job %s (%s) adalah HARI INI.", j.JobCode, j.Title)
	case scheduled.Equal(tomorrow):
		return "Pengingat: Jadwal Servis Anda Besok",
			fmt.Sprintf("Jadwal servis untuk job %s (%s) adalah BESOK (%s).",
				j.JobCode, j.Title, scheduled.Format("2 January 2006"))
	default:
		return "", ""
	}
}

// businessTimezone = WIB (UTC+7, tidak ada DST) - dipakai FixedZone, bukan
// time.LoadLocation("Asia/Jakarta"), supaya tidak bergantung ke IANA tzdata
// yang belum tentu ter-install di image Docker (Alpine base minimal sering
// tidak menyertakannya kecuali paket tzdata ditambahkan eksplisit).
var businessTimezone = time.FixedZone("WIB", 7*60*60)

// truncateToDate mengambil tanggal kalender (Y-M-D) dalam zona waktu bisnis
// (WIB), BUKAN zona waktu lokal proses Go maupun UTC mentah - ini penting:
// scheduled_date datang dari kolom DATE Postgres sebagai UTC tengah malam
// (mis. "2026-07-31" tersimpan sebagai 2026-07-31T00:00:00Z), sedangkan
// "today" dari time.Now() memakai zona waktu lokal server (bisa apa saja,
// tergantung OS/deployment). Membandingkan keduanya tanpa menyamakan zona
// dulu menghasilkan bug nyata yang ketemu saat verifikasi manual: job
// dengan scheduled_date besok tidak terklasifikasi "besok" sama sekali,
// karena "besok" versi UTC dan "besok" versi lokal +07 jatuh di instant
// yang beda. Menyamakan keduanya ke WIB (dipilih karena bisnis ini
// bengkel tunggal di Indonesia) membuat perbandingan tanggal kalender
// benar terlepas dari timezone proses Go yang menjalankannya.
func truncateToDate(t time.Time) time.Time {
	y, m, d := t.In(businessTimezone).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, businessTimezone)
}
