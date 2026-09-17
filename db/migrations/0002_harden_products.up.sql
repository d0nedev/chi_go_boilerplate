ALTER TABLE products
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at TYPE TIMESTAMPTZ USING updated_at AT TIME ZONE 'UTC';

ALTER TABLE products
    ADD CONSTRAINT products_price_non_negative CHECK (price >= 0),
    ADD CONSTRAINT products_name_not_blank CHECK (length(btrim(name)) > 0);

CREATE INDEX products_created_at_id_idx ON products (created_at DESC, id DESC);
