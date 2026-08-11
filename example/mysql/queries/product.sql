-- name: GetProduct :one
SELECT * FROM products WHERE id = ? LIMIT 1;

-- name: ListProducts :many
SELECT * FROM products ORDER BY created_at DESC, id DESC;

-- name: CreateProduct :execlastid
INSERT INTO products (name, price, stock)
VALUES (?, ?, ?);

-- name: UpdateProductStock :exec
UPDATE products SET stock = ? WHERE id = ?;

-- name: DeleteProduct :exec
DELETE FROM products WHERE id = ?;

-- name: GetProductPrice :one
SELECT price FROM products WHERE id = ? LIMIT 1;

-- name: GetProductsInStock :one
SELECT stock FROM products WHERE stock > 0 LIMIT 1;
