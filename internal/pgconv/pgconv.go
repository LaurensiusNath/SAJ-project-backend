// Package pgconv membungkus konversi antara tipe domain biasa (*string,
// uuid.UUID, *time.Time) dan tipe pgx/pgtype (pgtype.Text, pgtype.UUID, dst).
// Dipakai oleh repository.go di setiap modul, supaya detail nullable-nya
// Postgres tidak ditulis ulang di tiap modul.
package pgconv

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func ToText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func FromText(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

func ToUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

// ToNullableUUID sama seperti ToUUID, tapi untuk kolom FK yang boleh NULL
// (mis. jobs.machine_id sebelum mesin ditentukan).
func ToNullableUUID(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

func FromUUID(id pgtype.UUID) uuid.UUID {
	return uuid.UUID(id.Bytes)
}

func FromNullableUUID(id pgtype.UUID) *uuid.UUID {
	if !id.Valid {
		return nil
	}
	v := uuid.UUID(id.Bytes)
	return &v
}

func ToDate(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

func FromDate(t pgtype.Date) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// FromTimestamptz dipakai untuk kolom timestamptz yang NOT NULL (mis.
// created_at) - selalu Valid, jadi caller tidak perlu tangani nil.
func FromTimestamptz(t pgtype.Timestamptz) time.Time {
	return t.Time
}

func FromNullableTimestamptz(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}
