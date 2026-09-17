DROP INDEX IF EXISTS products_created_at_id_idx;

ALTER TABLE products
    DROP CONSTRAINT IF EXISTS products_name_not_blank,
    DROP CONSTRAINT IF EXISTS products_price_non_negative;

ALTER TABLE products
    ALTER COLUMN created_at TYPE TIMESTAMP USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at TYPE TIMESTAMP USING updated_at AT TIME ZONE 'UTC';
