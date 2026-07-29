package job

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/nathan/cnc-pm-backend/internal/pgconv"
	"github.com/nathan/cnc-pm-backend/internal/repository/sqlcgen"
)

type CostRepository interface {
	Create(ctx context.Context, c JobCost) (JobCost, error)
	ListByJob(ctx context.Context, jobID uuid.UUID) ([]JobCost, error)
	Totals(ctx context.Context, jobID uuid.UUID) (CostTotals, error)
	Delete(ctx context.Context, jobID, costID uuid.UUID) error
}

type sqlcCostRepository struct {
	q *sqlcgen.Queries
}

func NewCostRepository(q *sqlcgen.Queries) CostRepository {
	return &sqlcCostRepository{q: q}
}

func (r *sqlcCostRepository) Create(ctx context.Context, c JobCost) (JobCost, error) {
	row, err := r.q.CreateJobCost(ctx, sqlcgen.CreateJobCostParams{
		JobID:         pgconv.ToUUID(c.JobID),
		CostType:      string(c.CostType),
		Description:   c.Description,
		Quantity:      pgconv.ToNumeric(c.Quantity),
		PurchasePrice: pgconv.ToNullableNumeric(c.PurchasePrice),
		SellingPrice:  pgconv.ToNumeric(c.SellingPrice),
	})
	if err != nil {
		// job_id menunjuk ke job yang tidak ada -> foreign key violation,
		// sama seperti asInvalidReference di repository.go (satu package).
		if mapped := asInvalidReference(err); mapped != nil {
			return JobCost{}, mapped
		}
		return JobCost{}, fmt.Errorf("insert job cost: %w", err)
	}
	return fromCostRow(row), nil
}

func (r *sqlcCostRepository) ListByJob(ctx context.Context, jobID uuid.UUID) ([]JobCost, error) {
	rows, err := r.q.ListJobCostsByJob(ctx, pgconv.ToUUID(jobID))
	if err != nil {
		return nil, fmt.Errorf("list job costs: %w", err)
	}
	costs := make([]JobCost, len(rows))
	for i, row := range rows {
		costs[i] = fromCostRow(row)
	}
	return costs, nil
}

func (r *sqlcCostRepository) Totals(ctx context.Context, jobID uuid.UUID) (CostTotals, error) {
	row, err := r.q.JobCostTotals(ctx, pgconv.ToUUID(jobID))
	if err != nil {
		return CostTotals{}, fmt.Errorf("job cost totals: %w", err)
	}
	return CostTotals{
		TotalSelling: pgconv.FromNumeric(row.TotalSelling),
		TotalMargin:  pgconv.FromNumeric(row.TotalMargin),
	}, nil
}

func (r *sqlcCostRepository) Delete(ctx context.Context, jobID, costID uuid.UUID) error {
	rowsAffected, err := r.q.DeleteJobCost(ctx, sqlcgen.DeleteJobCostParams{
		ID:    pgconv.ToUUID(costID),
		JobID: pgconv.ToUUID(jobID),
	})
	if err != nil {
		return fmt.Errorf("delete job cost: %w", err)
	}
	if rowsAffected == 0 {
		return ErrCostNotFound
	}
	return nil
}

func fromCostRow(row sqlcgen.JobCost) JobCost {
	return JobCost{
		ID:            pgconv.FromUUID(row.ID),
		JobID:         pgconv.FromUUID(row.JobID),
		CostType:      CostType(row.CostType),
		Description:   row.Description,
		Quantity:      pgconv.FromNumeric(row.Quantity),
		PurchasePrice: pgconv.FromNullableNumeric(row.PurchasePrice),
		SellingPrice:  pgconv.FromNumeric(row.SellingPrice),
		Subtotal:      pgconv.FromNumeric(row.Subtotal),
		CreatedAt:     pgconv.FromTimestamptz(row.CreatedAt),
	}
}
