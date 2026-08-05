package taxreport

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
// jadi default awal-akhir bulan berjalan. Sama persis pola
// dashboard.GetSummaryParams.
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

// resolvePeriod adalah DUPLIKASI SADAR dari dashboard.resolvePeriod
// (logic sama persis: default awal-akhir bulan berjalan per-field
// independen, truncate ke tanggal murni, ToExclusive = To+1hari) - BUKAN
// di-reuse/diekspos dari package dashboard. Dua alasan:
//
//  1. Loose coupling antar modul bisnis: dashboard dan taxreport adalah
//     dua modul satelit SEJAJAR (bukan salah satu bergantung ke yang
//     lain secara alami) - meng-import dashboard dari sini cuma untuk satu
//     fungsi kecil melanggar prinsip "modul bisnis tidak saling import
//     langsung antar domain" yang sudah dipegang project ini, padahal
//     fungsi ini sendiri sama sekali tidak spesifik ke dashboard (tidak
//     nyentuh tipe/logic dashboard apapun selain nama parameter).
//  2. Konsisten dengan preseden yang SUDAH diambil project ini untuk
//     kasus yang persis sama (duplikasi function kecil lintas modul):
//     parseOptionalDate+dateLayout SUDAH terduplikasi 3x (job, invoice,
//     dashboard - lihat Technical Debt item #10 di api-contract.md),
//     sengaja DIBIARKAN alih-alih diekstrak, supaya tiap PR fitur tetap
//     scoped dan tidak menyeret perubahan ke modul yang sudah stabil.
//
// Ini SEKARANG jadi duplikasi ke-2 (dashboard + taxreport) untuk pola
// "resolve period dari query param" - kalau nanti ada modul KETIGA yang
// butuh pola sama, itu titik yang tepat untuk benar-benar diekstrak ke
// package baru (mis. internal/period) - dicatat sebagai item Technical
// Debt terpisah di docs/api-contract.md, bukan diekstrak diam-diam di
// PR ini.
func resolvePeriod(params GetSummaryParams, now time.Time) (PeriodInput, error) {
	from := params.From
	to := params.To

	if from == nil || to == nil {
		startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
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
