package settings

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRepository - satu baris saja (singleton), diinisialisasi dengan nilai
// default yang masuk akal, sama seperti migration 000007 men-seed baris
// asli di database.
type fakeRepository struct {
	current CompanySettings
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		current: CompanySettings{
			CompanyName:          "PT Servis CNC Sejahtera",
			IsPKP:                true,
			DefaultTaxPercentage: decimal.NewFromInt(11),
			DefaultPPh23Rate:     decimal.NewFromInt(2),
		},
	}
}

func (f *fakeRepository) Get(_ context.Context) (CompanySettings, error) {
	return f.current, nil
}

func (f *fakeRepository) Update(_ context.Context, s CompanySettings) (CompanySettings, error) {
	f.current = s
	return f.current, nil
}

func TestService_Get(t *testing.T) {
	svc := NewService(newFakeRepository())

	got, err := svc.Get(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "PT Servis CNC Sejahtera", got.CompanyName)
}

func TestService_Update(t *testing.T) {
	testCases := []struct {
		name    string
		input   UpdateInput
		wantErr error
	}{
		{
			name: "valid update",
			input: UpdateInput{
				CompanyName: "PT Baru", IsPKP: true,
				DefaultTaxPercentage: decimal.NewFromInt(12), DefaultPPh23Rate: decimal.NewFromInt(2),
			},
		},
		{
			name: "empty company_name is rejected",
			input: UpdateInput{
				CompanyName: "", IsPKP: true,
				DefaultTaxPercentage: decimal.NewFromInt(11), DefaultPPh23Rate: decimal.NewFromInt(2),
			},
			wantErr: ErrInvalidCompanyName,
		},
		{
			name: "negative tax percentage is rejected",
			input: UpdateInput{
				CompanyName: "PT Baru", IsPKP: true,
				DefaultTaxPercentage: decimal.NewFromInt(-1), DefaultPPh23Rate: decimal.NewFromInt(2),
			},
			wantErr: ErrInvalidPercentage,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(newFakeRepository())

			got, err := svc.Update(context.Background(), tc.input)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.input.CompanyName, got.CompanyName)
		})
	}
}
