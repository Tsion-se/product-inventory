-- Indexes are dropped with the table; the explicit drop keeps the intent clear.
-- No custom types are created by the up migration, so there is nothing else to remove.
DROP INDEX IF EXISTS idx_products_status;
DROP TABLE IF EXISTS products;
