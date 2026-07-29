package invoice

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// TestComputeAmounts_ExampleFromContract menggunakan angka persis dari
// contoh response POST /jobs/{id}/invoice di docs/api-contract.md, supaya
// implementasi ComputeAmounts bisa langsung dicocokkan ke dokumen kontrak,
// bukan cuma "kelihatan masuk akal".
func TestComputeAmounts_ExampleFromContract(t *testing.T) {
	subtotalAll := decimal.NewFromInt(1500000)
	dppPph23 := decimal.NewFromInt(800000)
	taxPercentage := decimal.NewFromInt(11)
	pph23Rate := decimal.NewFromInt(2)

	got := ComputeAmounts(subtotalAll, dppPph23, taxPercentage, pph23Rate, true)

	assert.True(t, got.TaxAmount.Equal(decimal.NewFromInt(165000)), "tax_amount, got %s", got.TaxAmount)
	assert.True(t, got.Total.Equal(decimal.NewFromInt(1665000)), "total, got %s", got.Total)
	assert.True(t, got.PPh23EstimatedAmount.Equal(decimal.NewFromInt(16000)), "pph23_estimated_amount, got %s", got.PPh23EstimatedAmount)
	assert.True(t, got.ExpectedReceivable.Equal(decimal.NewFromInt(1649000)), "expected_receivable, got %s", got.ExpectedReceivable)
}

// TestComputeAmounts_PeroranganCustomerSkipsPPh23 - customer perorangan
// tidak wajib memotong PPh 23 (lihat api-contract.md modul Customer), jadi
// pph23_estimated_amount harus 0 dan expected_receivable = total penuh,
// walau dpp_pph23 (basisnya) tetap ada.
func TestComputeAmounts_PeroranganCustomerSkipsPPh23(t *testing.T) {
	got := ComputeAmounts(
		decimal.NewFromInt(1500000),
		decimal.NewFromInt(800000),
		decimal.NewFromInt(11),
		decimal.NewFromInt(2),
		false,
	)

	assert.True(t, got.PPh23EstimatedAmount.IsZero(), "pph23_estimated_amount should be zero for perorangan, got %s", got.PPh23EstimatedAmount)
	assert.True(t, got.ExpectedReceivable.Equal(got.Total), "expected_receivable should equal total when no PPh23 is withheld")
}

func TestComputeAmounts_ZeroDPPPPh23YieldsZeroEstimate(t *testing.T) {
	got := ComputeAmounts(
		decimal.NewFromInt(1500000),
		decimal.Zero, // job cuma berisi spare_part, tidak ada labor/transport
		decimal.NewFromInt(11),
		decimal.NewFromInt(2),
		true,
	)

	assert.True(t, got.PPh23EstimatedAmount.IsZero(), "pph23_estimated_amount should be zero when dpp_pph23 is zero")
}
