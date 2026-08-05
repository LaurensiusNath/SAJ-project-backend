package taxreport

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRepository di sini SENGAJA tidak menyentuh Postgres - unit test ini
// cuma menguji resolusi period dan alur Service, sama pola dengan
// dashboard/service_test.go. Untuk bukti angka agregat dan filtering yang
// benar terhadap data sungguhan, lihat integration_test.go.
type fakeRepository struct {
	lastPeriod PeriodInput
	called     bool
	summary    Summary
	err        error
}

func (f *fakeRepository) GetSummary(_ context.Context, period PeriodInput) (Summary, error) {
	f.called = true
	f.lastPeriod = period
	if f.err != nil {
		return Summary{}, f.err
	}
	return f.summary, nil
}

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestResolvePeriod_DefaultsToCurrentMonth(t *testing.T) {
	now := date(2026, time.August, 15)

	got, err := resolvePeriod(GetSummaryParams{}, now)

	require.NoError(t, err)
	assert.True(t, got.From.Equal(date(2026, time.August, 1)), "From = %v", got.From)
	assert.True(t, got.To.Equal(date(2026, time.August, 31)), "To = %v", got.To)
	assert.True(t, got.ToExclusive.Equal(date(2026, time.September, 1)), "ToExclusive = %v", got.ToExclusive)
}

func TestResolvePeriod_DefaultsToCurrentMonth_FebruaryLeapYear(t *testing.T) {
	testCases := []struct {
		name     string
		now      time.Time
		wantLast int
	}{
		{name: "2026 - not a leap year", now: date(2026, time.February, 10), wantLast: 28},
		{name: "2028 - leap year", now: date(2028, time.February, 10), wantLast: 29},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolvePeriod(GetSummaryParams{}, tc.now)

			require.NoError(t, err)
			assert.Equal(t, tc.wantLast, got.To.Day())
		})
	}
}

func TestResolvePeriod_ExplicitRangeIsUsedAsIs(t *testing.T) {
	from := date(2026, time.March, 5)
	to := date(2026, time.March, 20)

	got, err := resolvePeriod(GetSummaryParams{From: &from, To: &to}, date(2026, time.August, 1))

	require.NoError(t, err)
	assert.True(t, got.From.Equal(from))
	assert.True(t, got.To.Equal(to))
	assert.True(t, got.ToExclusive.Equal(date(2026, time.March, 21)), "ToExclusive must be To+1 day so the entire last day is included")
}

func TestResolvePeriod_InvalidRange(t *testing.T) {
	from := date(2026, time.August, 20)
	to := date(2026, time.August, 10)

	_, err := resolvePeriod(GetSummaryParams{From: &from, To: &to}, date(2026, time.August, 1))

	require.ErrorIs(t, err, ErrInvalidPeriod)
}

func TestService_GetSummary_InvalidPeriod_RepositoryNeverCalled(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo)
	from := date(2026, time.August, 20)
	to := date(2026, time.August, 10)

	_, err := svc.GetSummary(context.Background(), GetSummaryParams{From: &from, To: &to})

	require.ErrorIs(t, err, ErrInvalidPeriod)
	assert.False(t, repo.called, "repository must not be queried when the period itself is invalid")
}

func TestService_GetSummary_PropagatesRepositoryResult(t *testing.T) {
	want := Summary{PPN: PPNSummary{TotalPPNKeluaran: decimal.NewFromInt(1000)}}
	repo := &fakeRepository{summary: want}
	svc := NewService(repo)

	got, err := svc.GetSummary(context.Background(), GetSummaryParams{})

	require.NoError(t, err)
	assert.True(t, got.PPN.TotalPPNKeluaran.Equal(want.PPN.TotalPPNKeluaran))
}
