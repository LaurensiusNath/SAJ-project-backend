package job

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// CostType merepresentasikan kolom job_costs.cost_type.
type CostType string

const (
	CostTypeLabor     CostType = "labor"
	CostTypeSparePart CostType = "spare_part"
	CostTypeTransport CostType = "transport"
	CostTypeOther     CostType = "other"
)

func (t CostType) Valid() bool {
	switch t {
	case CostTypeLabor, CostTypeSparePart, CostTypeTransport, CostTypeOther:
		return true
	default:
		return false
	}
}

var (
	ErrCostNotFound            = errors.New("job cost not found")
	ErrInvalidCostType         = errors.New("cost_type must be one of: labor, spare_part, transport, other")
	ErrInvalidQuantity         = errors.New("quantity must be greater than zero")
	ErrInvalidSellingPrice     = errors.New("selling_price must not be negative")
	ErrPurchasePriceNotAllowed = errors.New("purchase_price is only allowed for cost_type spare_part")
)

// JobCost adalah satu baris biaya di dalam sebuah Job. Quantity/PurchasePrice/
// SellingPrice/Subtotal memakai decimal.Decimal, bukan float64 - lihat
// internal/pgconv/pgconv.go untuk alasannya (presisi uang).
//
// Subtotal tidak pernah diisi manual di sini - ia selalu hasil baca balik
// dari kolom generated di database (lihat migration 000006), jadi field ini
// cuma pernah diisi oleh repository.go, tidak pernah oleh CreateInput.
type JobCost struct {
	ID            uuid.UUID        `json:"id"`
	JobID         uuid.UUID        `json:"job_id"`
	CostType      CostType         `json:"cost_type"`
	Description   string           `json:"description"`
	Quantity      decimal.Decimal  `json:"quantity"`
	PurchasePrice *decimal.Decimal `json:"purchase_price"`
	SellingPrice  decimal.Decimal  `json:"selling_price"`
	Subtotal      decimal.Decimal  `json:"subtotal"`
	CreatedAt     time.Time        `json:"created_at"`
}

func (c JobCost) Validate() error {
	if !c.CostType.Valid() {
		return ErrInvalidCostType
	}
	if c.Quantity.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidQuantity
	}
	if c.SellingPrice.IsNegative() {
		return ErrInvalidSellingPrice
	}
	if c.PurchasePrice != nil && c.CostType != CostTypeSparePart {
		return ErrPurchasePriceNotAllowed
	}
	return nil
}

// CostTotals dipakai untuk meta.total_selling / meta.total_margin di
// response GET /jobs/{id}/costs.
type CostTotals struct {
	TotalSelling decimal.Decimal
	TotalMargin  decimal.Decimal
}
