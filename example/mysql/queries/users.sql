-- name: GetUser :one
SELECT * FROM users WHERE id = ? LIMIT 1;

-- name: ListUsers :many
SELECT * FROM users WHERE name IN (sqlc.slice(names)) ORDER BY name;

-- name: CreateUser :execlastid
INSERT INTO users (name, email, phone)
VALUES (?, ?, ?);

-- name: DeleteUser :exec
DELETE FROM users WHERE id = ?;

-- name: UpdateUser :exec
UPDATE users
SET name = ?, email = ?
WHERE id = ?;

-- name: CountUsers :one
SELECT COUNT(*) FROM users;
