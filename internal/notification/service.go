package notification

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Sender adalah interface kecil yang dipakai modul LAIN (job.Service, dst)
// untuk bergantung ke notification tanpa perlu tahu konstruksi *Service
// yang sebenarnya (Repository + Mailer) - sama pola dengan Repository di
// modul lain, supaya caller-nya bisa diuji pakai fake yang cuma
// mengimplementasikan method ini.
type Sender interface {
	SendEmail(ctx context.Context, in EmailInput) error
}

type Service struct {
	repo   Repository
	mailer Mailer
}

func NewService(repo Repository, mailer Mailer) *Service {
	return &Service{repo: repo, mailer: mailer}
}

type EmailInput struct {
	To        string
	Subject   string
	Body      string
	JobID     *uuid.UUID
	InvoiceID *uuid.UUID
}

// SendEmail mengirim lalu MENCATAT hasilnya (berhasil atau gagal), tidak
// pernah dirancang untuk membatalkan operasi bisnis yang memanggilnya.
// Caller (job.Service.AssignTechnician, dst) sengaja mengabaikan error
// balikan ini sebagai alasan menggagalkan request-nya sendiri - notifikasi
// adalah efek samping, bukan bagian dari korektnes data inti, beda dari
// invoice.CreateFromJob yang wajib atomic. Error tetap dikembalikan supaya
// caller BISA log kalau mau (mis. log.Printf), bukan supaya request-nya
// ikut gagal.
func (s *Service) SendEmail(ctx context.Context, in EmailInput) error {
	status := StatusSent
	var errMsg *string

	sendErr := s.mailer.Send(in.To, in.Subject, in.Body)
	if sendErr != nil {
		status = StatusFailed
		msg := sendErr.Error()
		errMsg = &msg
	}

	subject := in.Subject
	_, logErr := s.repo.Create(ctx, Notification{
		Channel:      ChannelEmail,
		Recipient:    in.To,
		Subject:      &subject,
		Message:      in.Body,
		Status:       status,
		ErrorMessage: errMsg,
		JobID:        in.JobID,
		InvoiceID:    in.InvoiceID,
	})
	if logErr != nil {
		if sendErr != nil {
			return fmt.Errorf("send email failed (%w) and logging that failure also failed: %v", sendErr, logErr)
		}
		return fmt.Errorf("log notification: %w", logErr)
	}

	return sendErr
}
