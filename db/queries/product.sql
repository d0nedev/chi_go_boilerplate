-- name: ListProducts :many
SELECT
    id,
    name,
    price,
    created_at,
    updated_at
FROM products
WHERE sqlc.narg('cursor_created_at')::timestamptz IS NULL
   OR (created_at, id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('row_limit');

-- name: GetProduct :one
SELECT 
    id, 
    name, 
    price, 
    created_at, 
    updated_at 
FROM products 
WHERE id = $1;

-- name: CreateProduct :one
INSERT INTO products (
    name,
    price
)
VALUES (
    $1,
    $2
)
RETURNING
    id,
    name,
    price,
    created_at,
    updated_at;

-- name: UpdateProduct :one
UPDATE products
SET
    name = $2,
    price = $3,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING
    id,
    name,
    price,
    created_at,
    updated_at;

-- name: DeleteProduct :one
DELETE FROM products
WHERE id = $1
RETURNING id;