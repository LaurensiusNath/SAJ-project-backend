package notification

import (
	"fmt"
	"net/smtp"
)

// Mailer adalah abstraksi pengiriman email - Service bergantung ke
// interface ini, bukan net/smtp langsung, supaya bisa diuji pakai fake
// (sama pola dengan Repository di modul manapun di project ini).
type Mailer interface {
	Send(to, subject, body string) error
}

type smtpMailer struct {
	addr string
	from string
	auth smtp.Auth
}

// NewSMTPMailer pakai PLAIN AUTH biasa (username/password) - cukup untuk
// SMTP provider umum (Gmail App Password, Zoho, dst). host+port digabung
// jadi satu "addr" ("smtp.gmail.com:587") karena begitu format yang diminta
// smtp.SendMail.
func NewSMTPMailer(host, port, username, password, from string) Mailer {
	return &smtpMailer{
		addr: host + ":" + port,
		from: from,
		auth: smtp.PlainAuth("", username, password, host),
	}
}

func (m *smtpMailer) Send(to, subject, body string) error {
	// net/smtp cuma menyediakan transport, bukan builder pesan - header dan
	// body harus dirakit manual sesuai format RFC 5322 (tiap baris header
	// diakhiri CRLF, dipisah dari body oleh satu baris kosong).
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s\r\n", m.from, to, subject, body)

	if err := smtp.SendMail(m.addr, m.auth, m.from, []string{to}, []byte(msg)); err != nil {
		return fmt.Errorf("send email via smtp: %w", err)
	}
	return nil
}
