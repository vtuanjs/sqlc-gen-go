-- name: GetOrder :one
SELECT * FROM orders WHERE id = $1 LIMIT 1;

-- name: ListOrdersByUser :many
SELECT * FROM orders WHERE user_id = $1 ORDER BY created_at DESC;

-- name: CreateOrder :one
INSERT INTO orders (user_id, amount, status)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateOrderStatus :exec
UPDATE orders SET status = $2 WHERE id = $1;

-- name: GetUserOrderSummary :one
SELECT u.name, COUNT(o.id) as order_count, COALESCE(SUM(o.amount), 0) as total_spent
FROM users u
LEFT JOIN orders o ON o.user_id = u.id
WHERE u.id = $1
GROUP BY u.name;
