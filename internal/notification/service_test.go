package notification

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeMailer struct {
	shouldFail  bool
	sentTo      string
	sentSubject string
	sentBody    string
}

func (f *fakeMailer) Send(to, subject, body string) error {
	f.sentTo, f.sentSubject, f.sentBody = to, subject, body
	if f.shouldFail {
		return errors.New("smtp connection refused")
	}
	return nil
}

type fakeRepository struct {
	created []Notification
}

func (f *fakeRepository) Create(_ context.Context, n Notification) (Notification, error) {
	f.created = append(f.created, n)
	return n, nil
}

func (f *fakeRepository) ExistsSentToday(context.Context, uuid.UUID, string) (bool, error) {
	return false, nil
}

func TestService_SendEmail_Success(t *testing.T) {
	mailer := &fakeMailer{}
	repo := &fakeRepository{}
	svc := NewService(repo, mailer)

	err := svc.SendEmail(context.Background(), EmailInput{To: "test@example.com", Subject: "Halo", Body: "Isi pesan"})

	require.NoError(t, err)
	assert.Equal(t, "test@example.com", mailer.sentTo)
	assert.Equal(t, "Halo", mailer.sentSubject)
	require.Len(t, repo.created, 1)
	assert.Equal(t, StatusSent, repo.created[0].Status)
	assert.Nil(t, repo.created[0].ErrorMessage)
}

// TestService_SendEmail_MailerFailureIsLoggedNotDropped membuktikan sifat
// "fire-and-forget" yang diklaim di komentar service.go: kegagalan kirim
// TETAP tercatat di repository (bukan diam-diam hilang), dan errornya tetap
// dikembalikan ke caller supaya BISA di-log - tapi keputusan untuk tidak
// menggagalkan operasi bisnis ada di sisi caller (job.Service, dst), bukan
// di sini.
func TestService_SendEmail_MailerFailureIsLoggedNotDropped(t *testing.T) {
	mailer := &fakeMailer{shouldFail: true}
	repo := &fakeRepository{}
	svc := NewService(repo, mailer)

	err := svc.SendEmail(context.Background(), EmailInput{To: "test@example.com", Subject: "Halo", Body: "Isi pesan"})

	require.Error(t, err)
	require.Len(t, repo.created, 1, "a failed send must still be recorded, not silently dropped")
	assert.Equal(t, StatusFailed, repo.created[0].Status)
	require.NotNil(t, repo.created[0].ErrorMessage)
	assert.Contains(t, *repo.created[0].ErrorMessage, "smtp connection refused")
}
