-- name: GetProduct :one
SELECT * FROM products WHERE id = $1 LIMIT 1;

-- name: ListProducts :many
SELECT * FROM products ORDER BY created_at DESC;

-- name: CreateProduct :one
INSERT INTO products (name, price, stock)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateProductStock :exec
UPDATE products SET stock = $2 WHERE id = $1;

-- name: DeleteProduct :exec
DELETE FROM products WHERE id = $1;

-- name: GetProductPrice :one
SELECT price FROM products WHERE id = $1 LIMIT 1;

-- name: GetProductsInStock :one
SELECT stock FROM products WHERE stock > 0 LIMIT 1;