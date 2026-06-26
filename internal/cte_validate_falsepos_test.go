package golang

import "testing"

// TestValidateQueryCTEs_NoFalsePositives guards against the validator rejecting
// valid SQL because of constructs that look like — but are not — column
// references inside subquery conditions (type casts, function calls).
func TestValidateQueryCTEs_NoFalsePositives(t *testing.T) {
	tables := testCatalog()

	tests := []struct {
		name string
		sql  string
	}{
		{
			name: "cast to custom/enum type in subquery WHERE",
			sql: `WITH x AS (
				SELECT id, name FROM users u
				WHERE EXISTS (SELECT 1 FROM orders WHERE status = 'active'::order_status AND user_id = u.id)
			)
			SELECT x.id FROM x`,
		},
		{
			name: "custom function call in subquery WHERE",
			sql: `WITH x AS (
				SELECT id, name FROM users u
				WHERE EXISTS (SELECT 1 FROM orders WHERE my_func(amount) > 0 AND user_id = u.id)
			)
			SELECT x.id FROM x`,
		},
		{
			name: "column cast keeps the column checked",
			sql: `WITH x AS (
				SELECT id, name FROM users u
				WHERE EXISTS (SELECT 1 FROM orders WHERE amount::numeric > 0 AND user_id = u.id)
			)
			SELECT x.id FROM x`,
		},
		{
			name: "ILIKE with ESCAPE clause",
			sql: `WITH x AS (
				SELECT id, name FROM users u
				WHERE u.name ILIKE CONCAT('%', REPLACE(@q::text, '%', '\%'), '%') ESCAPE '\'
			)
			SELECT x.id FROM x`,
		},
		{
			name: "NOT LIKE / NOT ILIKE do not flag LIKE or ILIKE as a column",
			sql: `WITH x AS (
				SELECT id, name FROM users u
				WHERE u.name NOT LIKE '%foo%' AND u.email NOT ILIKE '%bar%'
			)
			SELECT x.id FROM x`,
		},
		{
			name: "NOT BETWEEN does not flag BETWEEN as a column",
			sql: `WITH x AS (
				SELECT id, user_id FROM orders o
				WHERE o.amount NOT BETWEEN 1 AND 100
			)
			SELECT x.id FROM x`,
		},
		{
			name: "CURRENT_DATE and CURRENT_TIMESTAMP as comparison values",
			sql: `WITH x AS (
				SELECT id, user_id FROM orders o
				WHERE o.created_at > CURRENT_DATE AND o.created_at < CURRENT_TIMESTAMP
			)
			SELECT x.id FROM x`,
		},
		{
			name: "AT TIME ZONE does not flag zone as a column",
			sql: `WITH x AS (
				SELECT id, user_id FROM orders o
				WHERE o.created_at AT TIME ZONE 'UTC' > CURRENT_TIMESTAMP
			)
			SELECT x.id FROM x`,
		},
		{
			name: "COLLATE in WHERE does not flag collate as a column",
			sql: `WITH x AS (
				SELECT id, name FROM users u
				WHERE u.name = 'foo' COLLATE "en-US"
			)
			SELECT x.id FROM x`,
		},
		{
			name: "COLLATE in ORDER BY does not flag collate as a column",
			sql: `WITH x AS (
				SELECT id, name FROM users u
				ORDER BY u.name COLLATE "en-US" ASC
			)
			SELECT x.id FROM x`,
		},
		{
			name: "USING in ORDER BY does not flag using as a column",
			sql: `WITH x AS (
				SELECT id, name FROM users u
				ORDER BY u.name USING <
			)
			SELECT x.id FROM x`,
		},
		{
			name: "CASE expression in WHERE does not flag case keywords as columns",
			sql: `WITH x AS (
				SELECT id, status FROM orders o
				WHERE CASE WHEN o.status = 'a' THEN 1 ELSE 2 END = 1
			)
			SELECT x.id FROM x`,
		},
		{
			name: "CASE expression in ORDER BY does not flag case keywords as columns",
			sql: `WITH x AS (
				SELECT id, status FROM orders o
				ORDER BY CASE WHEN o.status = 'a' THEN 1 ELSE 2 END
			)
			SELECT x.id FROM x`,
		},
		{
			name: "typed literal DATE on right of operator does not flag the type name",
			sql: `WITH x AS (
				SELECT id, created_at FROM orders o
				WHERE o.created_at > DATE '2020-01-01'
			)
			SELECT x.id FROM x`,
		},
		{
			name: "typed literal TIMESTAMP on right of operator does not flag the type name",
			sql: `WITH x AS (
				SELECT id, created_at FROM orders o
				WHERE o.created_at = TIMESTAMP '2020-01-01 00:00:00'
			)
			SELECT x.id FROM x`,
		},
		{
			name: "typed literal INTERVAL on right of operator does not flag the type name",
			sql: `WITH x AS (
				SELECT id, created_at FROM orders o
				WHERE o.created_at = INTERVAL '1 day'
			)
			SELECT x.id FROM x`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateQueryCTEs(tt.sql, tt.name, tables); err != nil {
				t.Errorf("false positive: %v", err)
			}
		})
	}
}

// TestValidateQueryCTEs_OrderGroupBy covers unqualified column references in
// ORDER BY / GROUP BY clauses, including SELECT-list aliases which must remain
// valid there.
func TestValidateQueryCTEs_OrderGroupBy(t *testing.T) {
	tables := testCatalog()

	tests := []struct {
		name    string
		sql     string
		wantErr bool
		errCol  string
	}{
		{
			name: "invalid column in subquery ORDER BY",
			sql: `WITH active_users AS (
				SELECT id, name, email
				FROM users u
				WHERE EXISTS (SELECT 1 FROM orders WHERE user_id = u.id ORDER BY created_at1 DESC LIMIT 1)
			)
			SELECT active_users.id FROM active_users`,
			wantErr: true,
			errCol:  "created_at1",
		},
		{
			name: "valid column in subquery ORDER BY",
			sql: `WITH active_users AS (
				SELECT id, name, email
				FROM users u
				WHERE EXISTS (SELECT 1 FROM orders WHERE user_id = u.id ORDER BY created_at DESC LIMIT 1)
			)
			SELECT active_users.id FROM active_users`,
			wantErr: false,
		},
		{
			name: "invalid column in CTE GROUP BY",
			sql: `WITH stats AS (
				SELECT user_id, COUNT(*) as cnt FROM orders GROUP BY user_id1
			)
			SELECT stats.user_id FROM stats`,
			wantErr: true,
			errCol:  "user_id1",
		},
		{
			name: "SELECT-list alias is valid in ORDER BY",
			sql: `WITH stats AS (
				SELECT user_id, COUNT(*) as cnt FROM orders GROUP BY user_id ORDER BY cnt DESC
			)
			SELECT stats.user_id FROM stats`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateQueryCTEs(tt.sql, tt.name, tables)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errCol != "" && !contains(err.Error(), tt.errCol) {
					t.Errorf("expected error to mention %q, got: %s", tt.errCol, err.Error())
				}
			} else if err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// TestValidateQueryCTEs_TopLevelWhere covers unqualified column references in a
// CTE body's own WHERE clause (not wrapped in a subquery).
func TestValidateQueryCTEs_TopLevelWhere(t *testing.T) {
	tables := testCatalog()

	tests := []struct {
		name    string
		sql     string
		wantErr bool
		errCol  string
	}{
		{
			name: "invalid unqualified column in CTE WHERE with EXCEPT",
			sql: `WITH high_value_users AS (
				SELECT DISTINCT user_id FROM orders WHERE amount1 > 100
				EXCEPT
				SELECT DISTINCT user_id FROM orders WHERE status = 'cancelled'
			)
			SELECT u.id, u.name, u.email
			FROM users u
			INNER JOIN high_value_users hvu ON hvu.user_id = u.id
			ORDER BY u.name`,
			wantErr: true,
			errCol:  "amount1",
		},
		{
			name: "valid unqualified columns in CTE WHERE with EXCEPT",
			sql: `WITH high_value_users AS (
				SELECT DISTINCT user_id FROM orders WHERE amount > 100
				EXCEPT
				SELECT DISTINCT user_id FROM orders WHERE status = 'cancelled'
			)
			SELECT u.id FROM users u
			INNER JOIN high_value_users hvu ON hvu.user_id = u.id`,
			wantErr: false,
		},
		{
			name: "EXISTS subquery does not leak into outer scope",
			sql: `WITH active_users AS (
				SELECT id, name, email
				FROM users u
				WHERE EXISTS (SELECT 1 FROM orders WHERE user_id = u.id)
			)
			SELECT active_users.id FROM active_users`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateQueryCTEs(tt.sql, tt.name, tables)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errCol != "" && !contains(err.Error(), tt.errCol) {
					t.Errorf("expected error to mention %q, got: %s", tt.errCol, err.Error())
				}
			} else if err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// TestValidateQueryCTEs_CastDoesNotHideBadColumn ensures stripping a cast still
// leaves the underlying column reference subject to validation.
func TestValidateQueryCTEs_CastDoesNotHideBadColumn(t *testing.T) {
	tables := testCatalog()
	sql := `WITH x AS (
		SELECT id, name FROM users u
		WHERE EXISTS (SELECT 1 FROM orders WHERE nonexistent::numeric > 0 AND user_id = u.id)
	)
	SELECT x.id FROM x`
	if err := validateQueryCTEs(sql, "bad", tables); err == nil {
		t.Fatal("expected error for nonexistent column, got nil")
	}
}
