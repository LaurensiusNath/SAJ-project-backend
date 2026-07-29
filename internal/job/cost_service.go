package job

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// CostService butuh Repository (job) selain CostRepository sendiri - supaya
// bisa memverifikasi job-nya benar-benar ada dulu sebelum create/list,
// dan mengembalikan ErrNotFound (job) yang jelas, bukan mengandalkan
// foreign key violation yang baru ketahuan setelah percobaan insert.
type CostService struct {
	repo    CostRepository
	jobRepo Repository
}

func NewCostService(repo CostRepository, jobRepo Repository) *CostService {
	return &CostService{repo: repo, jobRepo: jobRepo}
}

type CreateCostInput struct {
	JobID         uuid.UUID
	CostType      CostType
	Description   string
	Quantity      decimal.Decimal
	PurchasePrice *decimal.Decimal
	SellingPrice  decimal.Decimal
}

func (s *CostService) Create(ctx context.Context, in CreateCostInput) (JobCost, error) {
	if _, err := s.jobRepo.GetByID(ctx, in.JobID); err != nil {
		return JobCost{}, err
	}

	c := JobCost{
		JobID:         in.JobID,
		CostType:      in.CostType,
		Description:   in.Description,
		Quantity:      in.Quantity,
		PurchasePrice: in.PurchasePrice,
		SellingPrice:  in.SellingPrice,
	}
	if err := c.Validate(); err != nil {
		return JobCost{}, err
	}

	created, err := s.repo.Create(ctx, c)
	if err != nil {
		return JobCost{}, fmt.Errorf("create job cost: %w", err)
	}
	return created, nil
}

type CostListResult struct {
	Costs  []JobCost
	Totals CostTotals
}

func (s *CostService) ListByJob(ctx context.Context, jobID uuid.UUID) (CostListResult, error) {
	if _, err := s.jobRepo.GetByID(ctx, jobID); err != nil {
		return CostListResult{}, err
	}

	costs, err := s.repo.ListByJob(ctx, jobID)
	if err != nil {
		return CostListResult{}, fmt.Errorf("list job costs: %w", err)
	}
	totals, err := s.repo.Totals(ctx, jobID)
	if err != nil {
		return CostListResult{}, fmt.Errorf("job cost totals: %w", err)
	}
	return CostListResult{Costs: costs, Totals: totals}, nil
}

func (s *CostService) Delete(ctx context.Context, jobID, costID uuid.UUID) error {
	if err := s.repo.Delete(ctx, jobID, costID); err != nil {
		return fmt.Errorf("delete job cost: %w", err)
	}
	return nil
}
