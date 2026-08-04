package dashboard

import (
	"context"
	"time"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// GetSummaryParams adalah input opsional dari query string period_from/
// period_to - nil berarti "tidak diisi client", diterjemahkan resolvePeriod
// jadi default awal-akhir bulan berjalan.
type GetSummaryParams struct {
	From *time.Time
	To   *time.Time
}

func (s *Service) GetSummary(ctx context.Context, params GetSummaryParams) (Summary, error) {
	period, err := resolvePeriod(params, time.Now().UTC())
	if err != nil {
		return Summary{}, err
	}
	return s.repo.GetSummary(ctx, period)
}

// resolvePeriod menerjemahkan period_from/period_to (tanggal kalender,
// INKLUSIF kedua ujung, sesuai kontrak) jadi PeriodInput yang dipakai
// Repository - termasuk mengisi default "awal-akhir bulan berjalan" kalau
// salah satu/keduanya tidak diisi, dan menghitung ToExclusive (To + 1 hari)
// supaya seluruh hari terakhir period ikut ter-hitung di query DB
// (created_at pakai TIMESTAMPTZ, bukan DATE - kalau dibandingkan "<= To"
// begitu saja, baris yang created_at-nya jam 14:00 di hari To akan
// TERLEWAT karena To sendiri disimpan sebagai jam 00:00. Ini persis kelas
// bug off-by-one rentang tanggal yang diminta diwaspadai).
//
// now dijadikan PARAMETER (bukan langsung time.Now() di dalam fungsi)
// supaya fungsi ini murni dan gampang diuji terhadap tanggal spesifik tanpa
// perlu mocking clock - lihat service_test.go.
func resolvePeriod(params GetSummaryParams, now time.Time) (PeriodInput, error) {
	from := params.From
	to := params.To

	if from == nil || to == nil {
		startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		// AddDate(0, 1, -1) dari tanggal 1 = tanggal terakhir bulan yang
		// sama, otomatis benar untuk bulan 28/29/30/31 hari tanpa tabel
		// lookup manual (Go menormalkan tanggal "0 Maret" jadi "28/29
		// Februari" dst).
		endOfMonth := startOfMonth.AddDate(0, 1, -1)
		if from == nil {
			from = &startOfMonth
		}
		if to == nil {
			to = &endOfMonth
		}
	}

	if from.After(*to) {
		return PeriodInput{}, ErrInvalidPeriod
	}

	// Truncate ke tanggal murni (buang komponen jam-menit-detik kalau ada).
	// period_from/period_to dari query string sudah pasti date-only (di-parse
	// Handler pakai layout "2006-01-02", otomatis jadi midnight), tapi default
	// dari now (time.Now().UTC()) punya jam berjalan - wajib di-truncate di
	// sini juga supaya kedua jalur (diisi client vs default) menghasilkan
	// PeriodInput yang konsisten bentuknya.
	fromDate := truncateToDate(*from)
	toDate := truncateToDate(*to)

	return PeriodInput{
		From:        fromDate,
		To:          toDate,
		ToExclusive: toDate.AddDate(0, 0, 1),
	}, nil
}

func truncateToDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
