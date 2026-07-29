package job

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCostRepository struct {
	costs map[uuid.UUID]JobCost
}

func newFakeCostRepository() *fakeCostRepository {
	return &fakeCostRepository{costs: make(map[uuid.UUID]JobCost)}
}

func (f *fakeCostRepository) Create(_ context.Context, c JobCost) (JobCost, error) {
	c.ID = uuid.New()
	c.Subtotal = c.SellingPrice.Mul(c.Quantity)
	c.CreatedAt = time.Now()
	f.costs[c.ID] = c
	return c, nil
}

func (f *fakeCostRepository) ListByJob(_ context.Context, jobID uuid.UUID) ([]JobCost, error) {
	var result []JobCost
	for _, c := range f.costs {
		if c.JobID == jobID {
			result = append(result, c)
		}
	}
	return result, nil
}

func (f *fakeCostRepository) Totals(_ context.Context, jobID uuid.UUID) (CostTotals, error) {
	totalSelling := decimal.Zero
	totalMargin := decimal.Zero
	for _, c := range f.costs {
		if c.JobID != jobID {
			continue
		}
		totalSelling = totalSelling.Add(c.Subtotal)
		if c.CostType == CostTypeSparePart && c.PurchasePrice != nil {
			totalMargin = totalMargin.Add(c.SellingPrice.Sub(*c.PurchasePrice).Mul(c.Quantity))
		}
	}
	return CostTotals{TotalSelling: totalSelling, TotalMargin: totalMargin}, nil
}

func (f *fakeCostRepository) InvoiceTotals(_ context.Context, jobID uuid.UUID) (InvoiceCostTotals, error) {
	subtotalAll := decimal.Zero
	dppPph23 := decimal.Zero
	for _, c := range f.costs {
		if c.JobID != jobID {
			continue
		}
		subtotalAll = subtotalAll.Add(c.Subtotal)
		if c.CostType == CostTypeLabor || c.CostType == CostTypeTransport {
			dppPph23 = dppPph23.Add(c.Subtotal)
		}
	}
	return InvoiceCostTotals{SubtotalAll: subtotalAll, DPPPPh23: dppPph23}, nil
}

func (f *fakeCostRepository) Delete(_ context.Context, jobID, costID uuid.UUID) error {
	c, ok := f.costs[costID]
	if !ok || c.JobID != jobID {
		return ErrCostNotFound
	}
	delete(f.costs, costID)
	return nil
}

// seedJob menaruh satu Job langsung ke fakeRepository (job) supaya
// CostService.jobRepo.GetByID punya sesuatu untuk ditemukan - tidak lewat
// Service.Create supaya test ini tidak bergantung ke behavior job service.
func seedJob(t *testing.T, repo *fakeRepository) uuid.UUID {
	t.Helper()
	created, err := repo.Create(context.Background(), Job{
		CustomerID: uuid.New(),
		Title:      "Job untuk test cost",
		Status:     StatusRequested,
	})
	require.NoError(t, err)
	return created.ID
}

func TestCostService_Create(t *testing.T) {
	jobRepo := newFakeRepository()
	jobID := seedJob(t, jobRepo)
	purchasePrice := decimal.NewFromInt(120000)

	testCases := []struct {
		name    string
		input   CreateCostInput
		wantErr error
	}{
		{
			name: "valid spare_part with purchase_price",
			input: CreateCostInput{
				JobID: jobID, CostType: CostTypeSparePart, Description: "Bearing SKF 6205",
				Quantity: decimal.NewFromInt(2), PurchasePrice: &purchasePrice, SellingPrice: decimal.NewFromInt(150000),
			},
		},
		{
			name: "valid labor without purchase_price",
			input: CreateCostInput{
				JobID: jobID, CostType: CostTypeLabor, Description: "Jasa servis",
				Quantity: decimal.NewFromInt(1), SellingPrice: decimal.NewFromInt(200000),
			},
		},
		{
			name: "purchase_price on labor is rejected",
			input: CreateCostInput{
				JobID: jobID, CostType: CostTypeLabor, Description: "Jasa servis",
				Quantity: decimal.NewFromInt(1), PurchasePrice: &purchasePrice, SellingPrice: decimal.NewFromInt(200000),
			},
			wantErr: ErrPurchasePriceNotAllowed,
		},
		{
			name: "zero quantity is rejected",
			input: CreateCostInput{
				JobID: jobID, CostType: CostTypeTransport, Description: "Ongkos",
				Quantity: decimal.Zero, SellingPrice: decimal.NewFromInt(50000),
			},
			wantErr: ErrInvalidQuantity,
		},
		{
			name: "invalid cost_type is rejected",
			input: CreateCostInput{
				JobID: jobID, CostType: CostType("bukan-valid"), Description: "x",
				Quantity: decimal.NewFromInt(1), SellingPrice: decimal.NewFromInt(1000),
			},
			wantErr: ErrInvalidCostType,
		},
		{
			name: "nonexistent job returns not found",
			input: CreateCostInput{
				JobID: uuid.New(), CostType: CostTypeLabor, Description: "x",
				Quantity: decimal.NewFromInt(1), SellingPrice: decimal.NewFromInt(1000),
			},
			wantErr: ErrNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewCostService(newFakeCostRepository(), jobRepo)

			got, err := svc.Create(context.Background(), tc.input)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.True(t, got.Subtotal.Equal(tc.input.SellingPrice.Mul(tc.input.Quantity)),
				"subtotal should be selling_price * quantity")
		})
	}
}

func TestCostService_ListByJob_ComputesTotals(t *testing.T) {
	jobRepo := newFakeRepository()
	jobID := seedJob(t, jobRepo)
	costRepo := newFakeCostRepository()
	svc := NewCostService(costRepo, jobRepo)
	ctx := context.Background()
	purchasePrice := decimal.NewFromInt(120000)

	_, err := svc.Create(ctx, CreateCostInput{
		JobID: jobID, CostType: CostTypeSparePart, Description: "Bearing",
		Quantity: decimal.NewFromInt(2), PurchasePrice: &purchasePrice, SellingPrice: decimal.NewFromInt(150000),
	})
	require.NoError(t, err)
	_, err = svc.Create(ctx, CreateCostInput{
		JobID: jobID, CostType: CostTypeLabor, Description: "Jasa",
		Quantity: decimal.NewFromInt(1), SellingPrice: decimal.NewFromInt(200000),
	})
	require.NoError(t, err)

	result, err := svc.ListByJob(ctx, jobID)

	require.NoError(t, err)
	assert.Len(t, result.Costs, 2)
	// total_selling = (150000*2) + (200000*1) = 500000
	assert.True(t, result.Totals.TotalSelling.Equal(decimal.NewFromInt(500000)),
		"total_selling should sum all cost_type rows, got %s", result.Totals.TotalSelling)
	// total_margin cuma dari spare_part: (150000-120000)*2 = 60000
	assert.True(t, result.Totals.TotalMargin.Equal(decimal.NewFromInt(60000)),
		"total_margin should only count spare_part rows, got %s", result.Totals.TotalMargin)
}

func TestCostService_ListByJob_JobNotFound(t *testing.T) {
	svc := NewCostService(newFakeCostRepository(), newFakeRepository())

	_, err := svc.ListByJob(context.Background(), uuid.New())

	require.ErrorIs(t, err, ErrNotFound)
}

func TestCostService_Delete_NotFound(t *testing.T) {
	svc := NewCostService(newFakeCostRepository(), newFakeRepository())

	err := svc.Delete(context.Background(), uuid.New(), uuid.New())

	require.ErrorIs(t, err, ErrCostNotFound)
}
