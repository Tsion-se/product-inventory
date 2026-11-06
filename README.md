# Product Inventory API

A RESTful CRUD service for a product inventory, built with **Go**, **chi**, **sqlc (pgx/v5)**,
**PostgreSQL**, **golang-migrate** and **Docker Compose**.

## Features

- `products` table with check constraints, unique SKU and indexes (migration pair included)
- sqlc-generated, type-safe queries (`internal/product/queries.sql`)
- Layered design: **handler → service → repository → sqlc queries**
- Chi middleware: `RequestID`, `RealIP`, `Logger`, `Recoverer`, request timeout, JSON-only bodies
- Validation before any database access; uniform JSON error envelope
- Idempotent database seeder (14 sample products, all three statuses, in/low/out of stock)
- Multi-stage Docker build (static binary, Alpine runtime, non-root user) and Compose stack
- Bonus: `q` name search (ILIKE), `low_stock=true` filter, `GET /healthz`, automated tests, `requests.http`

## Project layout

```
cmd/api/                 API entry point
cmd/seeder/              Database seeder
internal/config/         Environment configuration
internal/database/       pgx connection pool
internal/httpx/          JSON/error helpers + middleware (RequireJSON, JSONRecoverer)
internal/product/        handler.go, service.go (validation), repository.go, model.go
internal/product/productdb/   sqlc generated code (do not edit)
internal/product/queries.sql  SQL queries for sqlc
internal/server/         Router, middleware wiring, /healthz
migrations/              golang-migrate .up.sql / .down.sql
sqlc.yaml  Makefile  Dockerfile  docker-compose.yml  requests.http  .env.example
```

## Prerequisites

| Tool | Needed for |
|------|-----------|
| Go 1.21+ | building/running/testing locally |
| Docker + Docker Compose v2 | the containerized stack |
| `sqlc` and `migrate` CLIs | `make sqlc`, `make migrate-*` on the host (`make tools` installs both) |
| `make` | the convenience targets |

## Quick start with Docker (recommended)

```bash
cp .env.example .env          # optional; defaults work without it
docker compose up --build     # starts PostgreSQL, applies migrations, starts the API
docker compose --profile tools run --rm seed   # load sample data (idempotent)

curl http://localhost:8080/healthz
curl "http://localhost:8080/api/v1/products?page=1&page_size=5"
```

`docker compose up` starts `db` first and waits for its `pg_isready` healthcheck, runs the one-shot
`migrate` service, and only then starts `api`. Data lives in the named volume `pgdata`, so it survives
`docker compose down` / `up`. Use `docker compose down -v` (or `make docker-reset`) to wipe it.

Equivalent make targets: `make docker-up`, `make docker-seed`, `make docker-down`, `make docker-migrate-down`.

## Local development (without the API in Docker)

```bash
cp .env.example .env
make tools                    # one-time: installs sqlc + migrate
docker compose up -d db       # PostgreSQL only (exposed on localhost:5432)

make tidy                     # first time only: resolves dependencies and writes go.sum
make migrate-up               # create the schema
make seed                     # insert sample data
make run                      # API on http://localhost:8080
```

Other targets: `make migrate-down` (rolls back one migration), `make sqlc`, `make build`,
`make test`, `make vet`, `make fmt`, `make check`. Run `make help` for the full list.

### Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_URL` | *(required)* | PostgreSQL connection string |
| `PORT` | `8080` | HTTP listen port |
| `ENV` | `development` | `production` switches logging to JSON |
| `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` | `inventory` | Compose database settings (dev defaults) |
| `DB_HOST_PORT` / `API_PORT` | `5432` / `8080` | Host ports published by Compose |

The default credentials are development placeholders. Set your own in `.env` (which is git-ignored)
for anything beyond local use.

## Data model

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | `BIGSERIAL` | primary key |
| `name` | `VARCHAR(255)` | required, not blank |
| `sku` | `VARCHAR(64)` | required, unique, `^[A-Z0-9]+(-[A-Z0-9]+)*$` |
| `description` | `TEXT` | nullable |
| `price` | `NUMERIC(10,2)` | required, `>= 0` |
| `stock_quantity` | `INTEGER` | required, default 0, `>= 0` |
| `status` | `VARCHAR(32)` | `draft` \| `active` \| `archived`, default `draft` |
| `created_at`, `updated_at` | `TIMESTAMPTZ` | default `NOW()` |

Indexes: the unique constraint on `sku` (backed by a unique btree index) and `idx_products_status`.

## API

Base path: `/api/v1/products`. Request bodies must be `application/json`.

| Method | Path | Success | Errors |
|--------|------|---------|--------|
| `POST` | `/` | `201` + product (`Location` header) | `400`, `409` (SKU exists), `415` |
| `GET` | `/` | `200` + `{data, pagination}` | `400` |
| `GET` | `/{id}` | `200` + product | `400` (bad id), `404` |
| `PUT` / `PATCH` | `/{id}` | `200` + product | `400`, `404`, `409`, `415` |
| `DELETE` | `/{id}` | `204` (empty body) | `400`, `404` |
| `GET` | `/healthz` | `200` `{"status":"ok","database":"up"}` | `503` when the DB is unreachable |

**List query parameters:** `page` (default 1), `page_size` or `limit` (default 10, values above 100 are
clamped to 100), `status` (`draft|active|archived`), `q` (case-insensitive name search),
`low_stock=true` (`stock_quantity <= 5`). Results are ordered newest first.

**Update semantics.** `PUT` and `PATCH` behave identically: only the fields present in the body change
(`name`, `sku`, `description`, `price`, `stock_quantity`, `status`); an empty body is rejected, and
`"description": ""` clears the description. Unknown fields (e.g. `id`, `created_at`) are ignored so a
`GET` payload can be sent back as-is. Changing `sku` to one that exists returns `409`.

**Price format.** Prices are exact decimals: up to 8 integer digits and 2 decimal places
(`19.99`, `5`, `0`). They are returned as JSON numbers with two decimals.

### Examples

```bash
# Create
curl -i -X POST http://localhost:8080/api/v1/products \
  -H 'Content-Type: application/json' \
  -d '{"name":"ProBook 15","sku":"LAP-PRO-15","price":1499.99,"stock_quantity":12,"status":"active"}'

# List page 1 of active products
curl "http://localhost:8080/api/v1/products?status=active&page=1&page_size=5"

# Get / update / delete
curl http://localhost:8080/api/v1/products/1
curl -X PATCH http://localhost:8080/api/v1/products/1 -H 'Content-Type: application/json' -d '{"stock_quantity":0}'
curl -i -X DELETE http://localhost:8080/api/v1/products/1
```

More samples are in [`requests.http`](requests.http).

### Response shapes

```jsonc
// Product
{ "id": 1, "name": "ProBook 15", "sku": "LAP-PRO-15", "description": null, "price": 1499.99,
  "stock_quantity": 12, "status": "active",
  "created_at": "2026-01-01T10:00:00Z", "updated_at": "2026-01-01T10:00:00Z" }

// GET /api/v1/products
{ "data": [ /* products */ ],
  "pagination": { "total_items": 15, "page": 2, "page_size": 5, "total_pages": 3 } }

// Every error
{ "error": "validation_failed", "message": "One or more fields are invalid.",
  "details": { "price": "must be greater than or equal to 0" } }
```

Error codes: `validation_failed`, `invalid_json`, `invalid_id`, `not_found`, `sku_conflict`,
`unsupported_media_type`, `method_not_allowed`, `timeout`, `internal_error`.
PostgreSQL unique violations (`23505`) map to `409`, `pgx.ErrNoRows` to `404`.

## Testing

```bash
make test        # go test -race ./...  (no database required)
make check       # gofmt check + go vet + tests
```

- `internal/server` - table-driven HTTP tests (`httptest`) covering every endpoint, validation,
  pagination, filters, error envelopes and `/healthz`, using an in-memory repository.
- `internal/httpx`, `internal/product` - unit tests for panic recovery, content-type checks,
  decimal handling and SKU/price validation.
- `internal/product/repository_integration_test.go` - runs the real SQL against PostgreSQL; it is
  skipped unless `TEST_DB_URL` is set:

  ```bash
  make migrate-up
  TEST_DB_URL="$DB_URL" go test ./internal/product -run Integration -v
  ```

## Regenerating sqlc code

Edit `internal/product/queries.sql` or the migrations, then run `make sqlc`. Never edit files in
`internal/product/productdb/` by hand.

## Notes

- The module path is `github.com/example/product-inventory`; rename it in `go.mod` and imports if needed.
- `go.sum` is created by `make tidy` (the Dockerfile also runs `go mod tidy`); commit it afterwards.
- Deletion is a hard delete.
