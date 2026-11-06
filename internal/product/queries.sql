-- name: CreateProduct :one
INSERT INTO products (name, sku, description, price, stock_quantity, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: CreateProductIfNotExists :execrows
-- Used by the seeder: idempotent insert, skipping SKUs that already exist.
INSERT INTO products (name, sku, description, price, stock_quantity, status)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (sku) DO NOTHING;

-- name: GetProductByID :one
SELECT * FROM products
WHERE id = $1;

-- name: GetProductBySKU :one
SELECT * FROM products
WHERE sku = $1;

-- name: ListProducts :many
-- Optional filters: status, case-insensitive name search, low stock (<= 5).
SELECT * FROM products
WHERE (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('search')::text IS NULL OR name ILIKE '%' || sqlc.narg('search') || '%')
  AND (NOT sqlc.arg('low_stock_only')::boolean OR stock_quantity <= 5)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('page_limit') OFFSET sqlc.arg('page_offset');

-- name: CountProducts :one
SELECT COUNT(*) FROM products
WHERE (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('search')::text IS NULL OR name ILIKE '%' || sqlc.narg('search') || '%')
  AND (NOT sqlc.arg('low_stock_only')::boolean OR stock_quantity <= 5);

-- name: UpdateProduct :one
UPDATE products
SET name           = $2,
    sku            = $3,
    description    = $4,
    price          = $5,
    stock_quantity = $6,
    status         = $7,
    updated_at     = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteProduct :exec
DELETE FROM products
WHERE id = $1;
