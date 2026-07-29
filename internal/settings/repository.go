package settings

import (
	"context"
	"fmt"

	"github.com/nathan/cnc-pm-backend/internal/pgconv"
	"github.com/nathan/cnc-pm-backend/internal/repository/sqlcgen"
)

type Repository interface {
	Get(ctx context.Context) (CompanySettings, error)
	Update(ctx context.Context, s CompanySettings) (CompanySettings, error)
}

type sqlcRepository struct {
	q *sqlcgen.Queries
}

func NewRepository(q *sqlcgen.Queries) Repository {
	return &sqlcRepository{q: q}
}

func (r *sqlcRepository) Get(ctx context.Context) (CompanySettings, error) {
	row, err := r.q.GetCompanySettings(ctx)
	if err != nil {
		return CompanySettings{}, fmt.Errorf("get company settings: %w", err)
	}
	return fromRow(row), nil
}

func (r *sqlcRepository) Update(ctx context.Context, s CompanySettings) (CompanySettings, error) {
	row, err := r.q.UpdateCompanySettings(ctx, sqlcgen.UpdateCompanySettingsParams{
		CompanyName:          s.CompanyName,
		Npwp:                 pgconv.ToText(s.NPWP),
		IsPkp:                s.IsPKP,
		DefaultTaxPercentage: pgconv.ToNumeric(s.DefaultTaxPercentage),
		DefaultPph23Rate:     pgconv.ToNumeric(s.DefaultPPh23Rate),
	})
	if err != nil {
		return CompanySettings{}, fmt.Errorf("update company settings: %w", err)
	}
	return fromRow(row), nil
}

func fromRow(row sqlcgen.CompanySetting) CompanySettings {
	return CompanySettings{
		CompanyName:          row.CompanyName,
		NPWP:                 pgconv.FromText(row.Npwp),
		IsPKP:                row.IsPkp,
		DefaultTaxPercentage: pgconv.FromNumeric(row.DefaultTaxPercentage),
		DefaultPPh23Rate:     pgconv.FromNumeric(row.DefaultPph23Rate),
		UpdatedAt:            pgconv.FromTimestamptz(row.UpdatedAt),
	}
}
