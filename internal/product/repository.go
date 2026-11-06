package product

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/example/product-inventory/internal/product/productdb"
)

// pgUniqueViolation is the PostgreSQL error code for unique_violation.
const pgUniqueViolation = "23505"

// Repository is the persistence boundary used by the Service.
type Repository interface {
	Create(ctx context.Context, p NewProduct) (Product, error)
	GetByID(ctx context.Context, id int64) (Product, error)
	GetBySKU(ctx context.Context, sku string) (Product, error)
	List(ctx context.Context, f Filter, limit, offset int32) ([]Product, error)
	Count(ctx context.Context, f Filter) (int64, error)
	Update(ctx context.Context, id int64, p NewProduct) (Product, error)
	Delete(ctx context.Context, id int64) error
}

// PgRepository implements Repository on top of the sqlc-generated queries.
type PgRepository struct {
	q *productdb.Queries
}

// NewPgRepository builds a repository on a pgx pool, connection or transaction.
func NewPgRepository(db productdb.DBTX) *PgRepository {
	return &PgRepository{q: productdb.New(db)}
}

var _ Repository = (*PgRepository)(nil)

func (r *PgRepository) Create(ctx context.Context, p NewProduct) (Product, error) {
	price, err := numericFromString(p.Price)
	if err != nil {
		return Product{}, err
	}
	row, err := r.q.CreateProduct(ctx, productdb.CreateProductParams{
		Name:          p.Name,
		Sku:           p.SKU,
		Description:   textFromPtr(p.Description),
		Price:         price,
		StockQuantity: p.StockQuantity,
		Status:        p.Status,
	})
	if err != nil {
		return Product{}, mapError(err)
	}
	return toDomain(row), nil
}

// SeedIfMissing inserts p unless its SKU already exists. It reports whether a
// row was inserted. It is used by the seeder and is not part of Repository.
func (r *PgRepository) SeedIfMissing(ctx context.Context, p NewProduct) (bool, error) {
	price, err := numericFromString(p.Price)
	if err != nil {
		return false, err
	}
	n, err := r.q.CreateProductIfNotExists(ctx, productdb.CreateProductIfNotExistsParams{
		Name:          p.Name,
		Sku:           p.SKU,
		Description:   textFromPtr(p.Description),
		Price:         price,
		StockQuantity: p.StockQuantity,
		Status:        p.Status,
	})
	if err != nil {
		return false, mapError(err)
	}
	return n > 0, nil
}

func (r *PgRepository) GetByID(ctx context.Context, id int64) (Product, error) {
	row, err := r.q.GetProductByID(ctx, id)
	if err != nil {
		return Product{}, mapError(err)
	}
	return toDomain(row), nil
}

func (r *PgRepository) GetBySKU(ctx context.Context, sku string) (Product, error) {
	row, err := r.q.GetProductBySKU(ctx, sku)
	if err != nil {
		return Product{}, mapError(err)
	}
	return toDomain(row), nil
}

func (r *PgRepository) List(ctx context.Context, f Filter, limit, offset int32) ([]Product, error) {
	rows, err := r.q.ListProducts(ctx, productdb.ListProductsParams{
		Status:       textFromPtr(f.Status),
		Search:       textFromPtr(f.Search),
		LowStockOnly: f.LowStock,
		PageLimit:    limit,
		PageOffset:   offset,
	})
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]Product, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDomain(row))
	}
	return out, nil
}

func (r *PgRepository) Count(ctx context.Context, f Filter) (int64, error) {
	n, err := r.q.CountProducts(ctx, productdb.CountProductsParams{
		Status:       textFromPtr(f.Status),
		Search:       textFromPtr(f.Search),
		LowStockOnly: f.LowStock,
	})
	if err != nil {
		return 0, mapError(err)
	}
	return n, nil
}

func (r *PgRepository) Update(ctx context.Context, id int64, p NewProduct) (Product, error) {
	price, err := numericFromString(p.Price)
	if err != nil {
		return Product{}, err
	}
	row, err := r.q.UpdateProduct(ctx, productdb.UpdateProductParams{
		ID:            id,
		Name:          p.Name,
		Sku:           p.SKU,
		Description:   textFromPtr(p.Description),
		Price:         price,
		StockQuantity: p.StockQuantity,
		Status:        p.Status,
	})
	if err != nil {
		return Product{}, mapError(err)
	}
	return toDomain(row), nil
}

func (r *PgRepository) Delete(ctx context.Context, id int64) error {
	return mapError(r.q.DeleteProduct(ctx, id))
}

// mapError translates database errors into domain errors:
// pgx.ErrNoRows -> ErrNotFound, SQLSTATE 23505 -> ErrSKUConflict.
func mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		return ErrSKUConflict
	}
	return err
}

func textFromPtr(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func toDomain(p productdb.Product) Product {
	var desc *string
	if p.Description.Valid {
		s := p.Description.String
		desc = &s
	}
	return Product{
		ID:            p.ID,
		Name:          p.Name,
		SKU:           p.Sku,
		Description:   desc,
		Price:         json.Number(numericToString(p.Price)),
		StockQuantity: p.StockQuantity,
		Status:        p.Status,
		CreatedAt:     p.CreatedAt.Time,
		UpdatedAt:     p.UpdatedAt.Time,
	}
}
