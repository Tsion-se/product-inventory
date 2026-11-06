// Command seeder populates the products table with sample data.
// It is idempotent: products whose SKU already exists are skipped.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/example/product-inventory/internal/config"
	"github.com/example/product-inventory/internal/database"
	"github.com/example/product-inventory/internal/product"
)

func main() {
	if err := run(); err != nil {
		slog.Error("seeding failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPool(ctx, cfg.DBURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	repo := product.NewPgRepository(pool)

	inserted, skipped := 0, 0
	for _, p := range seedProducts() {
		created, err := repo.SeedIfMissing(ctx, p)
		if err != nil {
			return fmt.Errorf("seed %s: %w", p.SKU, err)
		}
		if created {
			inserted++
		} else {
			skipped++
		}
	}
	fmt.Printf("Seeding complete: %d inserted, %d skipped (already present).\n", inserted, skipped)
	return nil
}

func ptr(s string) *string { return &s }

// seedProducts returns realistic sample data covering all three statuses,
// a range of prices, and in-stock, low-stock and out-of-stock items.
func seedProducts() []product.NewProduct {
	return []product.NewProduct{
		{Name: "ProBook 14 Laptop", SKU: "LAP-PRO-14", Description: ptr("14-inch laptop, 16 GB RAM, 512 GB SSD."), Price: "1299.00", StockQuantity: 25, Status: product.StatusActive},
		{Name: "AirBook 13 Ultralight", SKU: "LAP-AIR-13", Description: ptr("Ultralight 13-inch laptop with 18-hour battery life."), Price: "999.99", StockQuantity: 40, Status: product.StatusActive},
		{Name: "Nova X200 Smartphone", SKU: "PHN-X200", Description: ptr("6.5-inch OLED smartphone, 128 GB."), Price: "699.00", StockQuantity: 120, Status: product.StatusActive},
		{Name: "Nova Lite 5 Smartphone", SKU: "PHN-LITE-5", Description: ptr("Budget smartphone, launching next quarter."), Price: "249.50", StockQuantity: 0, Status: product.StatusDraft},
		{Name: "SoundWave Bluetooth Headphones", SKU: "AUD-BT-100", Description: ptr("Over-ear wireless headphones, 30-hour battery."), Price: "89.90", StockQuantity: 3, Status: product.StatusActive},
		{Name: "SilentPro ANC Earbuds", SKU: "AUD-ANC-300", Description: ptr("True wireless earbuds with active noise cancelling."), Price: "149.00", StockQuantity: 58, Status: product.StatusActive},
		{Name: "Mechanical Keyboard TKL", SKU: "ACC-KB-MECH", Description: ptr("Tenkeyless mechanical keyboard with hot-swappable switches."), Price: "79.00", StockQuantity: 4, Status: product.StatusActive},
		{Name: "Ergonomic Wireless Mouse", SKU: "ACC-MS-ERGO", Description: ptr("Vertical ergonomic mouse, rechargeable."), Price: "39.99", StockQuantity: 210, Status: product.StatusActive},
		{Name: "27-inch 4K Monitor", SKU: "MON-27-4K", Description: ptr("27-inch IPS 4K display with USB-C power delivery."), Price: "429.00", StockQuantity: 0, Status: product.StatusActive},
		{Name: "USB-C Cable 2m", SKU: "CAB-USBC-2M", Description: ptr("Braided USB-C to USB-C cable, 100 W."), Price: "12.99", StockQuantity: 500, Status: product.StatusActive},
		{Name: "ErgoFlex Office Chair", SKU: "OFF-CHAIR-01", Description: ptr("Adjustable ergonomic office chair, pending photography."), Price: "319.00", StockQuantity: 15, Status: product.StatusDraft},
		{Name: "Free Sample Sticker Pack", SKU: "SMP-STICKER-PACK", Price: "0.00", StockQuantity: 1000, Status: product.StatusDraft},
		{Name: "Tabula 9 Tablet (2019)", SKU: "TAB-OLD-9", Description: ptr("Discontinued 9-inch tablet."), Price: "129.00", StockQuantity: 0, Status: product.StatusArchived},
		{Name: "InkJet Printer 200", SKU: "PRN-INK-200", Description: ptr("Discontinued home inkjet printer; remaining stock only."), Price: "99.99", StockQuantity: 7, Status: product.StatusArchived},
	}
}
