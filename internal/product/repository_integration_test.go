package product_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/product-inventory/internal/product"
)

// TestPgRepositoryIntegration runs against a real PostgreSQL database that has
// the migrations applied. It is skipped unless TEST_DB_URL is set:
//
//	make migrate-up && TEST_DB_URL="$DB_URL" go test ./internal/product -run Integration
func TestPgRepositoryIntegration(t *testing.T) {
	url := os.Getenv("TEST_DB_URL")
	if url == "" {
		t.Skip("TEST_DB_URL not set; skipping PostgreSQL integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	repo := product.NewPgRepository(pool)

	sku := fmt.Sprintf("IT-%d", time.Now().UnixNano())
	desc := "integration test product"
	created, err := repo.Create(ctx, product.NewProduct{
		Name:          "Integration " + sku,
		SKU:           sku,
		Description:   &desc,
		Price:         "12.50",
		StockQuantity: 3,
		Status:        product.StatusActive,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = repo.Delete(context.Background(), created.ID) })

	if created.Price.String() != "12.50" || created.StockQuantity != 3 || created.Status != product.StatusActive {
		t.Fatalf("unexpected created product: %+v", created)
	}

	// Unique violation (23505) -> ErrSKUConflict.
	_, err = repo.Create(ctx, product.NewProduct{Name: "Dup", SKU: sku, Price: "1.00", Status: product.StatusDraft})
	if !errors.Is(err, product.ErrSKUConflict) {
		t.Fatalf("duplicate SKU error = %v, want ErrSKUConflict", err)
	}

	byID, err := repo.GetByID(ctx, created.ID)
	if err != nil || byID.SKU != sku {
		t.Fatalf("GetByID: %+v, %v", byID, err)
	}
	bySKU, err := repo.GetBySKU(ctx, sku)
	if err != nil || bySKU.ID != created.ID {
		t.Fatalf("GetBySKU: %+v, %v", bySKU, err)
	}

	updated, err := repo.Update(ctx, created.ID, product.NewProduct{
		Name: "Updated " + sku, SKU: sku, Price: "20.00", StockQuantity: 0, Status: product.StatusArchived,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Price.String() != "20.00" || updated.Status != product.StatusArchived ||
		updated.Description != nil || !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("unexpected updated product: %+v", updated)
	}

	status := product.StatusArchived
	search := strings.ToLower(sku) // ILIKE is case-insensitive
	filter := product.Filter{Status: &status, Search: &search, LowStock: true}
	n, err := repo.Count(ctx, filter)
	if err != nil || n != 1 {
		t.Fatalf("Count = %d, %v; want 1", n, err)
	}
	items, err := repo.List(ctx, filter, 10, 0)
	if err != nil || len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("List = %+v, %v", items, err)
	}

	if err := repo.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID(ctx, created.ID); !errors.Is(err, product.ErrNotFound) {
		t.Fatalf("GetByID after delete = %v, want ErrNotFound", err)
	}
	if _, err := repo.Update(ctx, created.ID, product.NewProduct{Name: "x", SKU: sku, Price: "1.00", Status: product.StatusDraft}); !errors.Is(err, product.ErrNotFound) {
		t.Fatalf("Update after delete = %v, want ErrNotFound", err)
	}
}
