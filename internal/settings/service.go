package settings

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Get(ctx context.Context) (CompanySettings, error) {
	return s.repo.Get(ctx)
}

// UpdateInput sengaja tidak punya UpdatedAt - itu selalu now() dari
// database, bukan input client, sama prinsipnya dengan CreatedAt di
// modul lain.
type UpdateInput struct {
	CompanyName          string
	NPWP                 *string
	IsPKP                bool
	DefaultTaxPercentage decimal.Decimal
	DefaultPPh23Rate     decimal.Decimal
}

func (s *Service) Update(ctx context.Context, in UpdateInput) (CompanySettings, error) {
	updated := CompanySettings{
		CompanyName:          in.CompanyName,
		NPWP:                 in.NPWP,
		IsPKP:                in.IsPKP,
		DefaultTaxPercentage: in.DefaultTaxPercentage,
		DefaultPPh23Rate:     in.DefaultPPh23Rate,
	}
	if err := updated.Validate(); err != nil {
		return CompanySettings{}, err
	}

	result, err := s.repo.Update(ctx, updated)
	if err != nil {
		return CompanySettings{}, fmt.Errorf("update company settings: %w", err)
	}
	return result, nil
}
