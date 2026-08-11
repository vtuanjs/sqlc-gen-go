-- name: GetOrder :one
SELECT * FROM orders WHERE id = ? LIMIT 1;

-- name: ListOrdersByUser :many
SELECT * FROM orders WHERE user_id = ? ORDER BY created_at DESC, id DESC;

-- name: CreateOrder :execlastid
INSERT INTO orders (user_id, amount, status)
VALUES (?, ?, ?);

-- name: UpdateOrderStatus :execrows
UPDATE orders SET status = ? WHERE id = ?;

-- name: GetUserOrderSummary :one
SELECT u.name, COUNT(o.id) AS order_count, COALESCE(SUM(o.amount), 0) AS total_spent
FROM users u
LEFT JOIN orders o ON o.user_id = u.id
WHERE u.id = ?
GROUP BY u.name;
