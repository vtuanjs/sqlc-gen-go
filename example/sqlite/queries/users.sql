-- name: GetUser :one
SELECT * FROM users WHERE id = ? LIMIT 1;

-- name: ListUsers :many
SELECT * FROM users WHERE name IN (sqlc.slice(names)) ORDER BY name;

-- name: CreateUser :one
INSERT INTO users (name, email, phone)
VALUES (?, ?, ?)
RETURNING *;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = ?;

-- name: UpdateUser :one
UPDATE users
SET name = ?, email = ?
WHERE id = ?
RETURNING *;

-- name: CountUsers :one
SELECT COUNT(*) FROM users;
