-- name: GetUserWithLock :one
SELECT *
FROM users
WHERE id = @id
LIMIT 1
FOR UPDATE -- :if $lock
;
