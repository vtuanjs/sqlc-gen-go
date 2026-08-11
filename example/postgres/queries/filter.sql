-- name: ListFilterItems :many
-- An optional filter in the middle of the WHERE clause: dropping @b renumbers
-- the placeholder for @c.
SELECT id, a, b, c
FROM filter_items
WHERE a = @a
  AND b = @b -- :if @b
  AND c = @c
ORDER BY id;

-- name: SearchFilterItems :many
SELECT id
FROM filter_items
WHERE kind = @kind
  AND id = ANY(@ids::bigint[]) -- :if @ids
ORDER BY id;
