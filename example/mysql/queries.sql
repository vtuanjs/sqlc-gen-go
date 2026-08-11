-- name: SearchFilterItems :many
SELECT id
FROM filter_items
WHERE kind = sqlc.arg(kind)
  AND id IN (sqlc.slice(ids)) -- :if @ids
