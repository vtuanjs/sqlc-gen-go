-- name: SearchUsers :many
SELECT * FROM users
WHERE name = @name
  -- :if @email
  AND email = @email
  AND phone = @phone -- :if @phone
  AND EXISTS ( -- :if @has_orders
    SELECT 1 FROM orders
    WHERE orders.user_id = users.id
      AND orders.created_at >= @orders_since -- :if @orders_since
  )
ORDER BY id ASC;

-- name: SearchUsersByContact :many
-- Include the combined contact filter only when BOTH email AND phone are provided.
SELECT * FROM users
WHERE name = @name
  AND (email = @email OR phone = @phone) -- :if @email @phone
ORDER BY id ASC;

-- name: SearchUsersOrdered :many
SELECT * FROM users
WHERE name = @name
  AND email = @email -- :if @email
ORDER BY
  created_at DESC, -- :if @order_created_at_desc
  name ASC, -- :if @order_name_asc
  id ASC;

-- name: SearchUsersOrderedByID :many
-- Every ORDER BY entry is removable. The static `TRUE` keeps the clause valid
-- whichever entries drop out, and doubles as the trailing static line sqlc's
-- SQLite parser needs: it discards a `-- :if` comment sitting on a statement's
-- last line, so an annotation must never be the final token before the `;`.
SELECT * FROM users
WHERE name = @name
  AND email = @email -- :if @email
ORDER BY
  id ASC,  -- :if @id_asc
  id DESC, -- :if @id_desc
  TRUE;

-- name: SearchUsersWithSameNameAndEmail :many
-- The same parameter gates (and fills) two conditions.
SELECT * FROM users
WHERE 1 = 1
  AND name = @name -- :if @name
  AND email = @name -- :if @name
ORDER BY id ASC;

-- name: SearchUsersWithBlock :many
SELECT * FROM users
WHERE 1 = 1
  AND ( -- :if @name
    name = @name
    AND email = @name
  )
ORDER BY id ASC;

-- name: SearchUsersWithTopStyle :many
SELECT * FROM users
WHERE 1 = 1
  -- :if @name
  AND (
    name = @name
    AND email = @name
  )
ORDER BY id ASC;

-- name: SearchUsersByIDs :many
-- Filter by a list of IDs. When ids is nil the condition is skipped and all
-- users matching the name are returned (nil slice = inactive filter).
SELECT * FROM users
WHERE name = @name
  AND id IN (sqlc.slice(ids)) -- :if @ids
ORDER BY id ASC;

-- name: SearchUsersWithPhone :many
-- SQLite has no FOR UPDATE, so this stands in for the postgres/mysql lock
-- example: a flag-only parameter that gates a clause without binding a value.
SELECT * FROM users
WHERE name = @name
  AND phone IS NOT NULL -- :if $with_phone
ORDER BY id ASC;

-- name: SearchUsersNestedOptional :many
-- A standalone `-- :if` nested inside an already-conditional block. The inner
-- condition gates only the line below it; dropping the outer one removes the
-- whole block, inner line included.
SELECT * FROM users
WHERE TRUE
  -- :if @email
  AND (
    email = @email
    -- :if $allow_no_phone
    OR phone IS NULL
  )
ORDER BY id ASC;

-- name: SearchUsersNestedBlock :many
-- The nested standalone annotation governs a whole multi-line sub-block, not
-- just the single line that follows it.
SELECT * FROM users
WHERE TRUE
  -- :if @name
  AND (
    name = @name
    -- :if $or_has_orders
    OR EXISTS (
      SELECT 1 FROM orders
      WHERE orders.user_id = users.id
    )
  )
ORDER BY id ASC;
