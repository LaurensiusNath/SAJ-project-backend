// Package dateonly menyediakan tipe Date untuk field yang cuma
// merepresentasikan tanggal kalender (tanpa komponen waktu/zona) - dipakai
// domain struct (Job.ScheduledDate, Job.CompletedDate, Invoice.DueDate,
// dashboard.Period) supaya JSON response-nya "YYYY-MM-DD", bukan RFC3339
// penuh yang dihasilkan encoding/json bawaan Go untuk time.Time telanjang.
//
// Root cause bug yang diperbaiki tipe ini: pgtype.Date (driver Postgres,
// lihat github.com/jackc/pgx/v5/pgtype/date.go) SEBENARNYA sudah benar
// sejak awal - dia implement MarshalJSON/UnmarshalJSON sendiri persis
// format date-only ("2006-01-02"). Masalahnya ada di internal/pgconv, yang
// mengonversi pgtype.Date ke *time.Time polos sebelum sampai ke domain -
// dan time.Time default marshal Go SELALU RFC3339 penuh, walau jamnya nol.
//
// Sengaja BUKAN pakai pgtype.Date langsung di domain/handler (lihat
// docs/api-contract.md Catatan Desain untuk pembahasan lengkap kenapa) -
// itu membocorkan tipe spesifik driver DB ke layer yang seharusnya tidak
// tahu apa-apa soal Postgres, dan nullable-nya pakai field Valid bool,
// bukan pointer-nil seperti konvensi SATU-SATUNYA yang dipakai di seluruh
// codebase ini. Date di package ini murni value type (setara uuid.UUID/
// decimal.Decimal yang sudah biasa dipakai langsung di domain struct),
// konversi ke/dari pgtype.Date ada di internal/pgconv (ToDate/FromDate).
package dateonly

import (
	"fmt"
	"time"
)

const layout = "2006-01-02"

// Date membungkus time.Time - method Marshal/UnmarshalJSON sendiri yang
// membedakannya dari time.Time polos.
//
// INVARIANT WAJIB, dijaga oleh SEMUA constructor di file ini (New/FromTime/
// Parse): Time SELALU Location=UTC dan jam-menit-detik=0. Ini mencegah
// kelas bug BEDA dari bug format JSON (pergeseran tanggal ±1 hari) yang
// bisa muncul kalau field Time diam-diam menyimpan instant di zona lain -
// lihat dateonly_test.go untuk bukti eksplisit round-trip dari zona
// non-UTC tidak menggeser tanggal.
type Date struct {
	time.Time
}

// New membuat Date dari komponen kalender langsung, selalu UTC.
func New(year int, month time.Month, day int) Date {
	return Date{time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}

// FromTime mengambil komponen kalender (Y/M/D) dari t APA ADANYA - di
// lokasi/zona t sendiri, BUKAN di-convert ke UTC dulu (itu justru bisa
// menggeser tanggal kalau t merepresentasikan waktu mepet tengah malam di
// zona aslinya) - lalu re-anchor hasilnya ke UTC tengah malam.
func FromTime(t time.Time) Date {
	y, m, d := t.Date()
	return New(y, m, d)
}

// Parse mem-parse string "YYYY-MM-DD" langsung - dipakai
// internal/dashboard/handler.go untuk query param (?period_from=...) yang
// datang sebagai string mentah lewat gin's c.Query(), BUKAN lewat JSON
// body binding, jadi tidak bisa otomatis lewat UnmarshalJSON.
func Parse(s string) (Date, error) {
	t, err := time.ParseInLocation(layout, s, time.UTC)
	if err != nil {
		return Date{}, fmt.Errorf("dateonly: invalid date %q, expected YYYY-MM-DD: %w", s, err)
	}
	return Date{t}, nil
}

// MarshalJSON pakai value receiver - berlaku otomatis baik untuk field
// bertipe Date maupun *Date (encoding/json menangani nil *Date sebagai
// "null" tanpa pernah memanggil method ini sama sekali).
func (d Date) MarshalJSON() ([]byte, error) {
	return []byte(`"` + d.Time.Format(layout) + `"`), nil
}

// UnmarshalJSON pakai pointer receiver (wajib, perlu mutasi penerima).
func (d *Date) UnmarshalJSON(b []byte) error {
	s := string(b)
	if s == "null" {
		*d = Date{}
		return nil
	}
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return fmt.Errorf("dateonly: invalid date %s, expected \"YYYY-MM-DD\"", s)
	}
	t, err := time.ParseInLocation(layout, s[1:len(s)-1], time.UTC)
	if err != nil {
		return fmt.Errorf("dateonly: invalid date %s: %w", s, err)
	}
	d.Time = t
	return nil
}

// String selalu "YYYY-MM-DD" - berguna untuk logging/debugging tanpa
// harus format manual di tiap titik pakai.
func (d Date) String() string {
	return d.Time.Format(layout)
}
