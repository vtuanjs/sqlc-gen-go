-- name: SearchUsers :many
-- The MySQL engine has no `@name` parameter syntax, so these queries use
-- sqlc.arg()/sqlc.slice() while the dynamic-filter annotations still refer to
-- parameters by name (`-- :if @email`).
SELECT * FROM users
WHERE name = sqlc.arg(name)
  -- :if @email
  AND email = sqlc.arg(email)
  AND phone = sqlc.arg(phone) -- :if @phone
  AND EXISTS ( -- :if @has_orders
    SELECT 1 FROM orders
    WHERE orders.user_id = users.id
      AND orders.created_at >= sqlc.arg(orders_since) -- :if @orders_since
  )
ORDER BY id ASC;

-- name: SearchUsersByContact :many
-- Include the combined contact filter only when BOTH email AND phone are provided.
SELECT * FROM users
WHERE name = sqlc.arg(name)
  AND (email = sqlc.arg(email) OR phone = sqlc.arg(phone)) -- :if @email @phone
ORDER BY id ASC;

-- name: SearchUsersOrdered :many
SELECT * FROM users
WHERE name = sqlc.arg(name)
  AND email = sqlc.arg(email) -- :if @email
ORDER BY
  created_at DESC, -- :if @order_created_at_desc
  name ASC, -- :if @order_name_asc
  id ASC;

-- name: SearchUsersOrderedByID :many
SELECT * FROM users
WHERE name = sqlc.arg(name)
  AND email = sqlc.arg(email) -- :if @email
ORDER BY
  id ASC,  -- :if @id_asc
  id DESC  -- :if @id_desc
;

-- name: SearchUsersWithBlock :many
SELECT * FROM users
WHERE 1 = 1
  AND ( -- :if @name
    name = sqlc.arg(name)
    AND email = sqlc.arg(name)
  )
ORDER BY id ASC;

-- name: SearchUsersWithTopStyle :many
SELECT * FROM users
WHERE 1 = 1
  -- :if @name
  AND (
    name = sqlc.arg(name)
    AND email = sqlc.arg(name)
  )
ORDER BY id ASC;

-- name: SearchUsersByIDs :many
-- Filter by a list of IDs. When ids is nil the condition is skipped and all
-- users matching the name are returned (nil slice = inactive filter).
SELECT * FROM users
WHERE name = sqlc.arg(name)
  AND id IN (sqlc.slice(ids)) -- :if @ids
ORDER BY id ASC;

-- name: SearchUsersWithSameNameAndEmail :many
-- The same parameter gates (and fills) two conditions.
SELECT * FROM users
WHERE 1 = 1
  AND name = sqlc.arg(name) -- :if @name
  AND email = sqlc.arg(name) -- :if @name
ORDER BY id ASC;

-- name: SearchUsersWithPhone :many
-- A flag-only parameter: with_phone gates a clause that binds no value.
SELECT * FROM users
WHERE name = sqlc.arg(name)
  AND phone IS NOT NULL -- :if $with_phone
ORDER BY id ASC;
