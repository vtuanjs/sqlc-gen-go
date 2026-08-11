-- name: GetUserWithLock :one
SELECT *
FROM users
WHERE id = sqlc.arg(id)
LIMIT 1
FOR UPDATE -- :if $lock
;
