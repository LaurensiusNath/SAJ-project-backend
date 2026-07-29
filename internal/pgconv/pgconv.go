// Package pgconv membungkus konversi antara tipe domain biasa (*string,
// uuid.UUID, *time.Time) dan tipe pgx/pgtype (pgtype.Text, pgtype.UUID, dst).
// Dipakai oleh repository.go di setiap modul, supaya detail nullable-nya
// Postgres tidak ditulis ulang di tiap modul.
package pgconv

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
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

// ToTimestamptz dipakai untuk parameter query timestamptz yang NOT NULL,
// mis. expected_updated_at pada optimistic locking (lihat job.AssignTechnician).
func ToTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// ToNumeric/FromNumeric konversi decimal.Decimal <-> pgtype.Numeric tanpa
// pernah singgah ke float64 - keduanya sama-sama merepresentasikan angka
// sebagai (coefficient, exponent), jadi konversinya persis/lossless. Ini
// yang membuat decimal.Decimal aman dipakai untuk nilai uang: tidak ada
// float rounding error yang menyelinap lewat pgx.
func ToNumeric(d decimal.Decimal) pgtype.Numeric {
	return pgtype.Numeric{Int: d.Coefficient(), Exp: d.Exponent(), Valid: true}
}

func FromNumeric(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}

func ToNullableNumeric(d *decimal.Decimal) pgtype.Numeric {
	if d == nil {
		return pgtype.Numeric{}
	}
	return ToNumeric(*d)
}

func FromNullableNumeric(n pgtype.Numeric) *decimal.Decimal {
	if !n.Valid {
		return nil
	}
	v := decimal.NewFromBigInt(n.Int, n.Exp)
	return &v
}

func FromNullableTimestamptz(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}
