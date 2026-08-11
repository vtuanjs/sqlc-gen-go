-- name: ListFilterItems :many
-- An optional filter in the middle of the WHERE clause: dropping @b renumbers
-- the placeholder for @c.
SELECT id, a, b, c
FROM filter_items
WHERE a = sqlc.arg(a)
  AND b = sqlc.arg(b) -- :if @b
  AND c = sqlc.arg(c)
ORDER BY id;

-- name: SearchFilterItems :many
SELECT id
FROM filter_items
WHERE kind = sqlc.arg(kind)
  AND id IN (sqlc.slice(ids)) -- :if @ids
ORDER BY id;
