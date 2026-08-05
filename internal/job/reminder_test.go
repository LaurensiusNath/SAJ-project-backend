package job

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nathan/cnc-pm-backend/internal/customer"
	"github.com/nathan/cnc-pm-backend/internal/dateonly"
	"github.com/nathan/cnc-pm-backend/internal/notification"
)

type fakeNotificationRepo struct {
	sentToday map[string]bool
}

func newFakeNotificationRepo() *fakeNotificationRepo {
	return &fakeNotificationRepo{sentToday: make(map[string]bool)}
}

func (f *fakeNotificationRepo) Create(_ context.Context, n notification.Notification) (notification.Notification, error) {
	return n, nil
}

func (f *fakeNotificationRepo) ExistsSentToday(_ context.Context, jobID uuid.UUID, subject string) (bool, error) {
	return f.sentToday[jobID.String()+subject], nil
}

func (f *fakeNotificationRepo) markSent(jobID uuid.UUID, subject string) {
	f.sentToday[jobID.String()+subject] = true
}

func TestReminderContent_Classification(t *testing.T) {
	today := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	j := Job{JobCode: "JOB-2026-0001", Title: "Servis rutin"}

	testCases := []struct {
		name          string
		scheduledDate *dateonly.Date
		wantSubject   string
	}{
		{name: "nil scheduled_date needs no reminder", scheduledDate: nil, wantSubject: ""},
		{name: "overdue (yesterday)", scheduledDate: ptrDate(2026, 8, 9), wantSubject: "Pengingat: Jadwal Servis Anda Telah Lewat"},
		{name: "overdue (last week)", scheduledDate: ptrDate(2026, 8, 3), wantSubject: "Pengingat: Jadwal Servis Anda Telah Lewat"},
		{name: "today", scheduledDate: ptrDate(2026, 8, 10), wantSubject: "Pengingat: Jadwal Servis Anda Hari Ini"},
		{name: "tomorrow", scheduledDate: ptrDate(2026, 8, 11), wantSubject: "Pengingat: Jadwal Servis Anda Besok"},
		{name: "too far ahead needs no reminder yet", scheduledDate: ptrDate(2026, 8, 20), wantSubject: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			j.ScheduledDate = tc.scheduledDate
			subject, body := reminderContent(j, today)
			assert.Equal(t, tc.wantSubject, subject)
			if tc.wantSubject != "" {
				assert.Contains(t, body, j.JobCode)
			} else {
				assert.Empty(t, body)
			}
		})
	}
}

// TestReminderContent_CrossTimezone_MatchesGmailWIBScenario adalah
// regression test untuk bug nyata yang ditemukan saat verifikasi manual:
// scheduled_date tersimpan sebagai UTC tengah malam, tapi "today" biasanya
// datang dari time.Now() di zona waktu lokal proses (mis. WIB, UTC+7) -
// tanpa disamakan ke satu zona (lihat truncateToDate), job yang jadwalnya
// besok tidak pernah terklasifikasi "besok" karena instant "besok versi
// UTC" dan "besok versi +07" berbeda.
func TestReminderContent_CrossTimezone_MatchesGmailWIBScenario(t *testing.T) {
	wib := time.FixedZone("WIB", 7*60*60)
	// "today" jam 9 pagi WIB - representatif untuk proses yang jalan di
	// server dengan zona waktu lokal +07 (bukan UTC).
	today := time.Date(2026, 7, 30, 9, 0, 0, 0, wib)
	// scheduled_date besok, tersimpan sebagai UTC tengah malam - persis
	// seperti hasil parseOptionalDate/pgconv.ToDate untuk input "2026-07-31".
	tomorrowUTC := dateonly.New(2026, 7, 31)
	j := Job{JobCode: "JOB-2026-0007", Title: "Servis Besok Test", ScheduledDate: &tomorrowUTC}

	subject, _ := reminderContent(j, today)

	assert.Equal(t, "Pengingat: Jadwal Servis Anda Besok", subject)
}

func ptrDate(y int, m time.Month, d int) *dateonly.Date {
	d2 := dateonly.New(y, m, d)
	return &d2
}

func TestReminderService_CheckAndNotify(t *testing.T) {
	jobRepo := newFakeRepository()
	custEmail := "customer@example.com"
	custRepo := stubCustomerRepo{customer: customer.Customer{Email: &custEmail}}
	notificationRepo := newFakeNotificationRepo()
	notifier := &recordingNotifier{}
	svc := NewReminderService(jobRepo, custRepo, notifier, notificationRepo)
	ctx := context.Background()

	overdue, err := jobRepo.Create(ctx, Job{CustomerID: uuid.New(), Title: "Overdue job", Status: StatusScheduled})
	require.NoError(t, err)
	overdue.ScheduledDate = ptrDate(2020, 1, 1) // jauh di masa lalu relatif ke "today" nyata
	jobRepo.jobs[overdue.ID] = overdue

	tooFar, err := jobRepo.Create(ctx, Job{CustomerID: uuid.New(), Title: "Far future job", Status: StatusScheduled})
	require.NoError(t, err)
	future := dateonly.FromTime(time.Now().AddDate(0, 1, 0))
	tooFar.ScheduledDate = &future
	jobRepo.jobs[tooFar.ID] = tooFar

	require.NoError(t, svc.CheckAndNotify(ctx))

	require.Len(t, notifier.sent, 1, "only the overdue job should trigger a reminder, not the one a month away")
	assert.Equal(t, custEmail, notifier.sent[0].To)
	assert.Equal(t, &overdue.ID, notifier.sent[0].JobID)
}

func TestReminderService_CheckAndNotify_SkipsIfAlreadySentToday(t *testing.T) {
	jobRepo := newFakeRepository()
	custEmail := "customer@example.com"
	custRepo := stubCustomerRepo{customer: customer.Customer{Email: &custEmail}}
	notificationRepo := newFakeNotificationRepo()
	notifier := &recordingNotifier{}
	svc := NewReminderService(jobRepo, custRepo, notifier, notificationRepo)
	ctx := context.Background()

	overdue, err := jobRepo.Create(ctx, Job{CustomerID: uuid.New(), Title: "Overdue job", Status: StatusScheduled})
	require.NoError(t, err)
	overdue.ScheduledDate = ptrDate(2020, 1, 1)
	jobRepo.jobs[overdue.ID] = overdue
	notificationRepo.markSent(overdue.ID, "Pengingat: Jadwal Servis Anda Telah Lewat")

	require.NoError(t, svc.CheckAndNotify(ctx))

	assert.Empty(t, notifier.sent, "a reminder already sent today for this job+subject must not be sent again")
}

func TestReminderService_CheckAndNotify_SkipsCustomerWithoutEmail(t *testing.T) {
	jobRepo := newFakeRepository()
	notificationRepo := newFakeNotificationRepo()
	notifier := &recordingNotifier{}
	svc := NewReminderService(jobRepo, noopCustomerRepo{}, notifier, notificationRepo)
	ctx := context.Background()

	overdue, err := jobRepo.Create(ctx, Job{CustomerID: uuid.New(), Title: "Overdue job", Status: StatusScheduled})
	require.NoError(t, err)
	overdue.ScheduledDate = ptrDate(2020, 1, 1)
	jobRepo.jobs[overdue.ID] = overdue

	require.NoError(t, svc.CheckAndNotify(ctx))

	assert.Empty(t, notifier.sent, "customer lookup fails (noopCustomerRepo always returns ErrNotFound), so nothing should be sent")
}
