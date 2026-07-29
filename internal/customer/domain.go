package customer

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// CustomerType merepresentasikan nilai kolom customers.customer_type.
// Go tidak punya enum bawaan, jadi idiom umumnya: definisikan tipe baru
// di atas string, lalu batasi nilainya lewat konstanta + method Valid().
type CustomerType string

const (
	CustomerTypeBadanUsaha CustomerType = "badan_usaha"
	CustomerTypePerorangan CustomerType = "perorangan"
)

// Valid mengembalikan true kalau nilai CustomerType termasuk salah satu
// yang diperbolehkan. Dipanggil dari Customer.Validate(), bukan di sini,
// supaya pesan error bisa disatukan dengan validasi field lain.
func (t CustomerType) Valid() bool {
	switch t {
	case CustomerTypeBadanUsaha, CustomerTypePerorangan:
		return true
	default:
		return false
	}
}

// Sentinel errors: nilai error yang dideklarasikan sekali sebagai variabel
// package-level, supaya caller di layer atas (service, handler) bisa
// membandingkannya pakai errors.Is(err, customer.ErrNotFound) tanpa perlu
// tahu detail pesannya. Ini pola standar Go untuk error yang punya arti
// spesifik (bukan cuma pesan generik).
var (
	ErrNotFound    = errors.New("customer not found")
	ErrInvalidName = errors.New("customer name is required")
	ErrInvalidType = errors.New("customer_type must be badan_usaha or perorangan")
)

// Customer adalah entity domain - representasi bisnis dari satu customer,
// sengaja tidak memakai tipe milik driver database (mis. pgtype.UUID)
// supaya domain.go tidak tahu-menahu soal Postgres/sqlc. Field opsional
// (nullable di kolom DB) memakai pointer, bukan string kosong, supaya
// "tidak diisi" dan "diisi string kosong" tetap bisa dibedakan.
type Customer struct {
	ID           uuid.UUID
	Name         string
	CustomerType CustomerType
	Phone        *string
	Email        *string
	Address      *string
	CompanyName  *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
}

// Validate menegakkan invariant domain yang harus selalu benar untuk
// sebuah Customer, terlepas dari apakah datanya datang dari HTTP request
// atau dari baris database. Receiver "c Customer" (value, bukan pointer)
// karena method ini cuma membaca field, tidak mengubah apa pun.
func (c Customer) Validate() error {
	if c.Name == "" {
		return ErrInvalidName
	}
	if !c.CustomerType.Valid() {
		return ErrInvalidType
	}
	return nil
}

// IsDeleted memudahkan pengecekan soft-delete tanpa caller perlu tahu
// representasinya (pointer nil = belum dihapus).
func (c Customer) IsDeleted() bool {
	return c.DeletedAt != nil
}
