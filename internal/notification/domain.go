package notification

import (
	"time"

	"github.com/google/uuid"
)

// Channel dibatasi ke "email" untuk sekarang - WhatsApp lewat reseller
// (rencana awalnya Fonnte) ditunda karena layanannya sudah tidak bisa
// dipakai, penggantinya belum diputuskan. Enum ini tetap dipisah (bukan
// diasumsikan selalu email di kode manapun) supaya penambahan channel baru
// nanti tidak perlu bongkar struktur, cukup tambah nilai + migration baru.
type Channel string

const ChannelEmail Channel = "email"

// Status: cuma "sent" atau "failed" - berbeda dari kolom status di modul
// lain (Invoice.Status dst) yang punya banyak tahapan, karena pengiriman
// di sini sinkron (langsung dicoba saat dipanggil, bukan diantrekan), jadi
// hasilnya selalu diketahui seketika, tidak pernah "pending".
type Status string

const (
	StatusSent   Status = "sent"
	StatusFailed Status = "failed"
)

// Notification adalah satu baris log percobaan kirim (berhasil atau
// gagal) - lihat migration 000012 untuk alasan ini log, bukan
// antrian/outbox.
type Notification struct {
	ID           uuid.UUID  `json:"id"`
	Channel      Channel    `json:"channel"`
	Recipient    string     `json:"recipient"`
	Subject      *string    `json:"subject"`
	Message      string     `json:"message"`
	Status       Status     `json:"status"`
	ErrorMessage *string    `json:"error_message"`
	JobID        *uuid.UUID `json:"job_id"`
	InvoiceID    *uuid.UUID `json:"invoice_id"`
	CreatedAt    time.Time  `json:"created_at"`
}
