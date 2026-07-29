package customer

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRepository adalah implementasi Repository in-memory, khusus untuk test.
// Sengaja ditulis tangan (map biasa), bukan pakai mocking framework -
// idiom Go umumnya lebih suka fake sederhana daripada mock berbasis
// reflection: lebih mudah dibaca, dan test langsung memverifikasi
// perilaku (state akhir), bukan urutan pemanggilan method.
type fakeRepository struct {
	customers map[uuid.UUID]Customer
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{customers: make(map[uuid.UUID]Customer)}
}

func (f *fakeRepository) Create(_ context.Context, c Customer) (Customer, error) {
	c.ID = uuid.New()
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()
	f.customers[c.ID] = c
	return c, nil
}

func (f *fakeRepository) GetByID(_ context.Context, id uuid.UUID) (Customer, error) {
	c, ok := f.customers[id]
	if !ok {
		return Customer{}, ErrNotFound
	}
	return c, nil
}

func (f *fakeRepository) List(_ context.Context, _ *string, _, _ int32) ([]Customer, error) {
	result := make([]Customer, 0, len(f.customers))
	for _, c := range f.customers {
		result = append(result, c)
	}
	return result, nil
}

func (f *fakeRepository) Update(_ context.Context, c Customer) (Customer, error) {
	if _, ok := f.customers[c.ID]; !ok {
		return Customer{}, ErrNotFound
	}
	f.customers[c.ID] = c
	return c, nil
}

func (f *fakeRepository) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := f.customers[id]; !ok {
		return ErrNotFound
	}
	delete(f.customers, id)
	return nil
}

func TestService_Create(t *testing.T) {
	phone := "081234567890"

	testCases := []struct {
		name    string
		input   CreateInput
		wantErr error
	}{
		{
			name: "valid badan usaha",
			input: CreateInput{
				Name:         "PT Sumber Makmur",
				CustomerType: CustomerTypeBadanUsaha,
				Phone:        &phone,
			},
			wantErr: nil,
		},
		{
			name: "empty name is rejected",
			input: CreateInput{
				Name:         "",
				CustomerType: CustomerTypePerorangan,
			},
			wantErr: ErrInvalidName,
		},
		{
			name: "invalid customer_type is rejected",
			input: CreateInput{
				Name:         "Budi",
				CustomerType: "bukan-tipe-valid",
			},
			wantErr: ErrInvalidType,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(newFakeRepository())

			got, err := svc.Create(context.Background(), tc.input)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.NotEqual(t, uuid.Nil, got.ID, "repository should assign an ID")
			assert.Equal(t, tc.input.Name, got.Name)
		})
	}
}

func TestService_GetByID_NotFound(t *testing.T) {
	svc := NewService(newFakeRepository())

	_, err := svc.GetByID(context.Background(), uuid.New())

	require.ErrorIs(t, err, ErrNotFound)
}

func TestService_Update_NotFound(t *testing.T) {
	svc := NewService(newFakeRepository())

	_, err := svc.Update(context.Background(), UpdateInput{ID: uuid.New(), Name: "Nama Baru"})

	require.ErrorIs(t, err, ErrNotFound)
}

func TestService_Update_RejectsEmptyName(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	created, err := svc.Create(context.Background(), CreateInput{
		Name:         "PT Awal",
		CustomerType: CustomerTypeBadanUsaha,
	})
	require.NoError(t, err)

	_, err = svc.Update(context.Background(), UpdateInput{ID: created.ID, Name: ""})

	require.ErrorIs(t, err, ErrInvalidName)
}

func TestService_Delete_NotFound(t *testing.T) {
	svc := NewService(newFakeRepository())

	err := svc.Delete(context.Background(), uuid.New())

	require.ErrorIs(t, err, ErrNotFound)
}

func TestService_Delete_RemovesCustomer(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	created, err := svc.Create(context.Background(), CreateInput{
		Name:         "PT Akan Dihapus",
		CustomerType: CustomerTypePerorangan,
	})
	require.NoError(t, err)

	err = svc.Delete(context.Background(), created.ID)
	require.NoError(t, err)

	_, err = svc.GetByID(context.Background(), created.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}
