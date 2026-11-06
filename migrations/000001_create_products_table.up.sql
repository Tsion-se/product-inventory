-- Product inventory: products table.
CREATE TABLE products (
    id             BIGSERIAL      PRIMARY KEY,
    name           VARCHAR(255)   NOT NULL,
    sku            VARCHAR(64)    NOT NULL,
    description    TEXT,
    price          NUMERIC(10, 2) NOT NULL,
    stock_quantity INTEGER        NOT NULL DEFAULT 0,
    status         VARCHAR(32)    NOT NULL DEFAULT 'draft',
    created_at     TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ    NOT NULL DEFAULT NOW(),

    -- The UNIQUE constraint is backed by a unique btree index on sku, which
    -- also serves fast SKU lookups (GetProductBySKU).
    CONSTRAINT products_sku_key              UNIQUE (sku),
    CONSTRAINT products_name_not_blank       CHECK (char_length(btrim(name)) > 0),
    CONSTRAINT products_sku_format           CHECK (sku ~ '^[A-Z0-9]+(-[A-Z0-9]+)*$'),
    CONSTRAINT products_price_non_negative   CHECK (price >= 0),
    CONSTRAINT products_stock_non_negative   CHECK (stock_quantity >= 0),
    CONSTRAINT products_status_valid         CHECK (status IN ('draft', 'active', 'archived'))
);

-- Fast filtering by status (GET /products?status=...).
CREATE INDEX idx_products_status ON products (status);
