package settings

import (
	"errors"
	"time"

	"github.com/shopspring/decimal"
)

var (
	ErrInvalidCompanyName = errors.New("company_name is required")
	ErrInvalidPercentage  = errors.New("tax percentage must not be negative")
)

// CompanySettings adalah singleton - selalu ada tepat satu baris (lihat
// migration 000007), jadi tidak ada ID di sini sama sekali; tidak ada
// "yang mana" untuk dipilih.
type CompanySettings struct {
	CompanyName          string          `json:"company_name"`
	NPWP                 *string         `json:"npwp"`
	IsPKP                bool            `json:"is_pkp"`
	DefaultTaxPercentage decimal.Decimal `json:"default_tax_percentage"`
	DefaultPPh23Rate     decimal.Decimal `json:"default_pph23_rate"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

func (s CompanySettings) Validate() error {
	if s.CompanyName == "" {
		return ErrInvalidCompanyName
	}
	if s.DefaultTaxPercentage.IsNegative() || s.DefaultPPh23Rate.IsNegative() {
		return ErrInvalidPercentage
	}
	return nil
}
