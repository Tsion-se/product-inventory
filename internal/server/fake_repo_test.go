package server

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/example/product-inventory/internal/product"
)

// fakeRepo is an in-memory product.Repository that mimics the behaviour of the
// PostgreSQL implementation (unique SKUs, filters, ordering).
type fakeRepo struct {
	mu     sync.Mutex
	nextID int64
	items  map[int64]product.Product
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{nextID: 1, items: map[int64]product.Product{}}
}

var _ product.Repository = (*fakeRepo)(nil)

func (f *fakeRepo) skuTaken(sku string, exceptID int64) bool {
	for id, p := range f.items {
		if id != exceptID && p.SKU == sku {
			return true
		}
	}
	return false
}

func (f *fakeRepo) Create(_ context.Context, p product.NewProduct) (product.Product, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.skuTaken(p.SKU, 0) {
		return product.Product{}, product.ErrSKUConflict
	}
	now := time.Now().UTC()
	item := product.Product{
		ID:            f.nextID,
		Name:          p.Name,
		SKU:           p.SKU,
		Description:   p.Description,
		Price:         json.Number(p.Price),
		StockQuantity: p.StockQuantity,
		Status:        p.Status,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	f.items[item.ID] = item
	f.nextID++
	return item, nil
}

func (f *fakeRepo) GetByID(_ context.Context, id int64) (product.Product, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.items[id]
	if !ok {
		return product.Product{}, product.ErrNotFound
	}
	return p, nil
}

func (f *fakeRepo) GetBySKU(_ context.Context, sku string) (product.Product, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.items {
		if p.SKU == sku {
			return p, nil
		}
	}
	return product.Product{}, product.ErrNotFound
}

func (f *fakeRepo) matching(flt product.Filter) []product.Product {
	unescape := strings.NewReplacer(`\\`, `\`, `\%`, `%`, `\_`, `_`)
	var out []product.Product
	for _, p := range f.items {
		if flt.Status != nil && p.Status != *flt.Status {
			continue
		}
		if flt.Search != nil {
			needle := strings.ToLower(unescape.Replace(*flt.Search))
			if !strings.Contains(strings.ToLower(p.Name), needle) {
				continue
			}
		}
		if flt.LowStock && p.StockQuantity > 5 {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

func (f *fakeRepo) List(_ context.Context, flt product.Filter, limit, offset int32) ([]product.Product, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	all := f.matching(flt)
	start := int(offset)
	if start > len(all) {
		start = len(all)
	}
	end := start + int(limit)
	if end > len(all) {
		end = len(all)
	}
	return append([]product.Product{}, all[start:end]...), nil
}

func (f *fakeRepo) Count(_ context.Context, flt product.Filter) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return int64(len(f.matching(flt))), nil
}

func (f *fakeRepo) Update(_ context.Context, id int64, p product.NewProduct) (product.Product, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	existing, ok := f.items[id]
	if !ok {
		return product.Product{}, product.ErrNotFound
	}
	if f.skuTaken(p.SKU, id) {
		return product.Product{}, product.ErrSKUConflict
	}
	existing.Name = p.Name
	existing.SKU = p.SKU
	existing.Description = p.Description
	existing.Price = json.Number(p.Price)
	existing.StockQuantity = p.StockQuantity
	existing.Status = p.Status
	existing.UpdatedAt = time.Now().UTC()
	f.items[id] = existing
	return existing, nil
}

func (f *fakeRepo) Delete(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.items, id)
	return nil
}
