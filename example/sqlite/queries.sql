-- name: ListFilterItems :many
SELECT id, a, b, c
FROM filter_items
WHERE a = @a
  AND b = @b -- :if @b
  AND c = @c
ORDER BY id;
