package db

import (
	"slices"
	"testing"

	"example/dbpostgres"
	"example/dbsqlite"
)

func TestDynamicSQL(t *testing.T) {
	t.Run("PostgreSQLPlaceholders/NilCondition", func(t *testing.T) {
		// $1 required, $2 conditional (nil → line removed)
		query := "SELECT * FROM t\nWHERE a = $1\n  AND b = $2 -- :if $2"
		var b *string
		gotQuery, gotArgs := dbpostgres.DynamicSQL(query, []any{"hello", b})

		assertSQL(t, gotQuery, "SELECT * FROM t\nWHERE a = $1")
		if len(gotArgs) != 1 {
			t.Errorf("args len: got %d, want 1", len(gotArgs))
		}
		if gotArgs[0] != "hello" {
			t.Errorf("args[0]: got %v, want hello", gotArgs[0])
		}
	})

	t.Run("RemapsGaps", func(t *testing.T) {
		// $1 required, $2 conditional removed, $3 required → $3 renumbered to $2
		query := "SELECT * FROM t\nWHERE a = $1\n  AND b = $2 -- :if $2\n  AND c = $3"
		var b *string
		gotQuery, gotArgs := dbpostgres.DynamicSQL(query, []any{"a", b, "c"})

		assertSQL(t, gotQuery, "SELECT * FROM t\nWHERE a = $1\n  AND c = $2")
		if len(gotArgs) != 2 {
			t.Fatalf("args len: got %d, want 2", len(gotArgs))
		}
		if gotArgs[0] != "a" || gotArgs[1] != "c" {
			t.Errorf("args: got %v, want [a c]", gotArgs)
		}
	})

	t.Run("SQLitePlaceholders/NilCondition", func(t *testing.T) {
		// ?1 required, ?2 conditional removed, ?3 required → ?3 becomes $2.
		query := "SELECT * FROM t\nWHERE a = ?1\n  AND b = ?2 -- :if $2\n  AND c = ?3"
		var b *string
		gotQuery, gotArgs := dbsqlite.DynamicSQL(query, []any{"a", b, "c"})

		assertSQL(t, gotQuery, "SELECT * FROM t\nWHERE a = $1\n  AND c = $2")
		if !slices.Equal(gotArgs, []any{"a", "c"}) {
			t.Errorf("args: got %v, want [a c]", gotArgs)
		}
	})

	t.Run("SQLitePlaceholders/NonNilCondition", func(t *testing.T) {
		b := "active"
		query := "SELECT * FROM t\nWHERE a = ?1\n  AND b = ?2 -- :if $2\n  AND c = ?3"
		gotQuery, gotArgs := dbsqlite.DynamicSQL(query, []any{"a", &b, "c"})

		assertSQL(t, gotQuery, "SELECT * FROM t\nWHERE a = $1\n  AND b = $2\n  AND c = $3")
		if !slices.Equal(gotArgs, []any{"a", &b, "c"}) {
			t.Errorf("args: got %v, want [a %v c]", gotArgs, &b)
		}
	})

	t.Run("BareQuestionMark", func(t *testing.T) {
		// PostgreSQL '?' is operator text, never a bind marker — with or
		// without an adjacent digit — so both survive untouched.
		query := "SELECT * FROM t WHERE json ? 'key' AND data?1 = 'x' AND a = $1"
		gotQuery, gotArgs := dbpostgres.DynamicSQL(query, []any{"value"})

		assertSQL(t, gotQuery, "SELECT * FROM t WHERE json ? 'key' AND data?1 = 'x' AND a = $1")
		if !slices.Equal(gotArgs, []any{"value"}) {
			t.Errorf("args: got %v, want [value]", gotArgs)
		}
	})

	t.Run("SQLiteSqlcSlice/NilCondition", func(t *testing.T) {
		query := "SELECT * FROM t\nWHERE name = ?1\n  AND id IN (/*SLICE:ids*/?2) -- :if $2"
		var ids []int64
		gotQuery, gotArgs := dbsqlite.DynamicSQL(query, []any{"active", ids})

		assertSQL(t, gotQuery, "SELECT * FROM t\nWHERE name = $1")
		if !slices.Equal(gotArgs, []any{"active"}) {
			t.Errorf("args: got %v, want [active]", gotArgs)
		}
	})

	t.Run("SQLiteSqlcSlice/EmptyCondition", func(t *testing.T) {
		// Empty non-nil keeps the :if-gated clause and renders NULL: an empty
		// allow-list filters by the empty set (fail-closed), unlike nil.
		query := "SELECT * FROM t\nWHERE name = ?1\n  AND id IN (/*SLICE:ids*/?2) -- :if $2"
		gotQuery, gotArgs := dbsqlite.DynamicSQL(query, []any{"active", []int64{}})

		assertSQL(t, gotQuery, "SELECT * FROM t\nWHERE name = $1\n  AND id IN (NULL)")
		if !slices.Equal(gotArgs, []any{"active"}) {
			t.Errorf("args: got %v, want [active]", gotArgs)
		}
	})

	t.Run("SQLiteSqlcSlice/NilableSliceEmptyCondition", func(t *testing.T) {
		// NilableSlice converts empty to nil for callers who want "no values
		// supplied" to mean "don't filter": the clause is skipped.
		query := "SELECT * FROM t\nWHERE name = ?1\n  AND id IN (/*SLICE:ids*/?2) -- :if $2"
		gotQuery, gotArgs := dbsqlite.DynamicSQL(query, []any{"active", dbsqlite.NilableSlice([]int64{})})

		assertSQL(t, gotQuery, "SELECT * FROM t\nWHERE name = $1")
		if !slices.Equal(gotArgs, []any{"active"}) {
			t.Errorf("args: got %v, want [active]", gotArgs)
		}
	})

	t.Run("SQLiteSqlcSlice/ActiveCondition", func(t *testing.T) {
		query := "SELECT * FROM t\nWHERE name = ?1\n  AND id IN (/*SLICE:ids*/?2) -- :if $2"
		gotQuery, gotArgs := dbsqlite.DynamicSQL(query, []any{"active", []int64{7, 9}})

		assertSQL(t, gotQuery, "SELECT * FROM t\nWHERE name = $1\n  AND id IN ($2,$3)")
		if !slices.Equal(gotArgs, []any{"active", int64(7), int64(9)}) {
			t.Errorf("args: got %v, want [active 7 9]", gotArgs)
		}
	})

	t.Run("PostgreSQLSqlcSlice/ActiveCondition", func(t *testing.T) {
		query := "SELECT * FROM t\nWHERE name = $1\n  AND id IN (/*SLICE:ids*/$2) -- :if $2"
		gotQuery, gotArgs := dbpostgres.DynamicSQL(query, []any{"active", []int64{7, 9}})

		assertSQL(t, gotQuery, "SELECT * FROM t\nWHERE name = $1\n  AND id IN ($2,$3)")
		if !slices.Equal(gotArgs, []any{"active", int64(7), int64(9)}) {
			t.Errorf("args: got %v, want [active 7 9]", gotArgs)
		}
	})

	t.Run("PostgreSQLPlaceholders/NonNilCondition", func(t *testing.T) {
		// All lines kept → placeholders stay sequential, args unchanged
		status := "active"
		query := "SELECT * FROM t\nWHERE a = $1\n  AND b = $2 -- :if $2"
		gotQuery, gotArgs := dbpostgres.DynamicSQL(query, []any{"hello", &status})

		assertSQL(t, gotQuery, "SELECT * FROM t\nWHERE a = $1\n  AND b = $2")
		if len(gotArgs) != 2 {
			t.Fatalf("args len: got %d, want 2", len(gotArgs))
		}
	})

	t.Run("NoAnnotations", func(t *testing.T) {
		query := "SELECT * FROM t WHERE a = $1 AND b = $2"
		gotQuery, gotArgs := dbpostgres.DynamicSQL(query, []any{"x", "y"})

		assertSQL(t, gotQuery, "SELECT * FROM t WHERE a = $1 AND b = $2")
		if len(gotArgs) != 2 {
			t.Errorf("args len: got %d, want 2", len(gotArgs))
		}
	})

	// ORDER BY flag tests: $1=required WHERE param, $2/$3=bool flags for ORDER BY lines
	const orderByQuery = "SELECT * FROM t\nWHERE a = $1\nORDER BY\n  id ASC, -- :if $2\n  name ASC, -- :if $3\n  created_at DESC"

	t.Run("OrderBy/AllFlagsInactive", func(t *testing.T) {
		sql, args := dbpostgres.DynamicSQL(orderByQuery, []any{"x", false, false})
		assertSQL(t, sql, "SELECT * FROM t\nWHERE a = $1\nORDER BY\n  created_at DESC")
		// $2 and $3 are annotation-only flags → only $1 remains
		if len(args) != 1 {
			t.Errorf("args len: got %d, want 1", len(args))
		}
	})

	t.Run("OrderBy/FirstFlagActive", func(t *testing.T) {
		sql, args := dbpostgres.DynamicSQL(orderByQuery, []any{"x", true, false})
		assertSQL(t, sql, "SELECT * FROM t\nWHERE a = $1\nORDER BY\n  id ASC,\n  created_at DESC")
		// $2 is annotation-only → only $1 in args
		if len(args) != 1 {
			t.Errorf("args len: got %d, want 1", len(args))
		}
	})

	t.Run("OrderBy/AllFlagsActive", func(t *testing.T) {
		sql, args := dbpostgres.DynamicSQL(orderByQuery, []any{"x", true, true})
		assertSQL(t, sql, "SELECT * FROM t\nWHERE a = $1\nORDER BY\n  id ASC,\n  name ASC,\n  created_at DESC")
		// $2 and $3 are annotation-only flags → only $1 in args
		if len(args) != 1 {
			t.Errorf("args len: got %d, want 1", len(args))
		}
	})

	t.Run("OrderBy/RemovedWhenEmpty", func(t *testing.T) {
		query := "SELECT * FROM t\nWHERE a = $1\nORDER BY\n  id ASC, -- :if $2\n  id DESC -- :if $3"
		sql, args := dbpostgres.DynamicSQL(query, []any{"x", false, false})
		assertSQL(t, sql, "SELECT * FROM t\nWHERE a = $1")
		if len(args) != 1 {
			t.Errorf("args len: got %d, want 1", len(args))
		}
	})

	t.Run("OrderBy/KeptWhenHasContent", func(t *testing.T) {
		query := "SELECT * FROM t\nWHERE a = $1\nORDER BY\n  id ASC, -- :if $2\n  id DESC -- :if $3"
		sql, _ := dbpostgres.DynamicSQL(query, []any{"x", false, true})
		assertSQL(t, sql, "SELECT * FROM t\nWHERE a = $1\nORDER BY\n  id DESC")
	})

	t.Run("OrderBy/KeptWhenHasContentFirstItem", func(t *testing.T) {
		query := "SELECT * FROM t\nWHERE a = $1\nORDER BY\n  id ASC, -- :if $2\n  id DESC -- :if $3"
		sql, _ := dbpostgres.DynamicSQL(query, []any{"x", true, false})
		assertSQL(t, sql, "SELECT * FROM t\nWHERE a = $1\nORDER BY\n  id ASC")
	})

	const orderByOnlyConditionalQuery = "SELECT * FROM t\nWHERE a = $1\nORDER BY\n  id ASC, -- :if $2\n  name DESC, -- :if $3\n  created_at DESC -- :if $4"

	t.Run("OrderBy/OmitAllLines", func(t *testing.T) {
		sql, args := dbpostgres.DynamicSQL(orderByOnlyConditionalQuery, []any{"x", false, false, false})
		assertSQL(t, sql, "SELECT * FROM t\nWHERE a = $1")
		if len(args) != 1 {
			t.Errorf("args len: got %d, want 1", len(args))
		}
	})

	t.Run("OrderBy/ContainsSecondLine", func(t *testing.T) {
		sql, args := dbpostgres.DynamicSQL(orderByOnlyConditionalQuery, []any{"x", false, true, false})
		assertSQL(t, sql, "SELECT * FROM t\nWHERE a = $1\nORDER BY\n  name DESC")
		if len(args) != 1 {
			t.Errorf("args len: got %d, want 1", len(args))
		}
	})

	t.Run("OrderBy/RemoveAllLinesSimple", func(t *testing.T) {
		query := "SELECT * FROM t\nWHERE a = $1\nORDER BY\n  id ASC -- :if $2\n  id DESC -- :if $3"
		sql, args := dbpostgres.DynamicSQL(query, []any{"x", false, false})
		assertSQL(t, sql, "SELECT * FROM t\nWHERE a = $1")
		if len(args) != 1 {
			t.Errorf("args len: got %d, want 1", len(args))
		}
	})

	const orderByWithStaticLineQuery = "SELECT * FROM t\nWHERE a = $1\nORDER BY\n  id ASC, -- :if $2\n  name DESC, -- :if $3\n  created_at DESC"

	t.Run("OrderBy/BlockExistWithStaticLine", func(t *testing.T) {
		sql, args := dbpostgres.DynamicSQL(orderByWithStaticLineQuery, []any{"x", false, false})
		assertSQL(t, sql, "SELECT * FROM t\nWHERE a = $1\nORDER BY\n  created_at DESC")
		if len(args) != 1 {
			t.Errorf("args len: got %d, want 1", len(args))
		}
	})

	t.Run("OrderBy/BlockExistFirstConditionalAndStatic", func(t *testing.T) {
		sql, args := dbpostgres.DynamicSQL(orderByWithStaticLineQuery, []any{"x", true, false})
		assertSQL(t, sql, "SELECT * FROM t\nWHERE a = $1\nORDER BY\n  id ASC,\n  created_at DESC")
		if len(args) != 1 {
			t.Errorf("args len: got %d, want 1", len(args))
		}
	})

	const existsQuery = "SELECT * FROM t\nWHERE a = $1\n  AND EXISTS ( -- :if $2\n    SELECT 1 -- :if $2\n    FROM other -- :if $2\n    WHERE id = $2 -- :if $2\n  ) -- :if $2"

	t.Run("ExistsOperator", func(t *testing.T) {
		var b *int
		sql, args := dbpostgres.DynamicSQL(existsQuery, []any{"x", b})
		assertSQL(t, sql, "SELECT * FROM t\nWHERE a = $1")
		if len(args) != 1 {
			t.Errorf("args len: got %d, want 1", len(args))
		}
	})

	t.Run("ExistsOperatorActive", func(t *testing.T) {
		b := 100
		sql, args := dbpostgres.DynamicSQL(existsQuery, []any{"x", &b})
		assertSQL(t, sql, "SELECT * FROM t\nWHERE a = $1\n  AND EXISTS (\n    SELECT 1\n    FROM other\n    WHERE id = $2\n  )")
		if len(args) != 2 {
			t.Errorf("args len: got %d, want 2", len(args))
		}
	})

	t.Run("DollarWithoutDigit", func(t *testing.T) {
		// Ensures no infinite loop when '$' appears without a trailing digit.
		query := "SELECT * FROM t WHERE a = $1 AND b::text = $$hello$$"
		sql, args := dbpostgres.DynamicSQL(query, []any{"x"})
		assertSQL(t, sql, "SELECT * FROM t WHERE a = $1 AND b::text = $$hello$$")
		if len(args) != 1 {
			t.Errorf("args len: got %d, want 1", len(args))
		}
	})

	t.Run("OrphanedWHERE", func(t *testing.T) {
		// All WHERE conditions are conditional → WHERE keyword must be removed.
		query := "SELECT * FROM t\nWHERE\n  a = $1 -- :if $1\n  AND b = $2 -- :if $2"
		var a, b *string
		sql, args := dbpostgres.DynamicSQL(query, []any{a, b})
		assertSQL(t, sql, "SELECT * FROM t")
		if len(args) != 0 {
			t.Errorf("args len: got %d, want 0", len(args))
		}
	})

	t.Run("CascadingWHEREAndOrderBy", func(t *testing.T) {
		// All WHERE + all ORDER BY removed → both keywords must be cleaned.
		query := "SELECT * FROM t\nWHERE\n  a = $1 -- :if $1\nORDER BY\n  id ASC -- :if $2"
		var a *string
		sql, args := dbpostgres.DynamicSQL(query, []any{a, false})
		assertSQL(t, sql, "SELECT * FROM t")
		if len(args) != 0 {
			t.Errorf("args len: got %d, want 0", len(args))
		}
	})

	t.Run("OrphanedGROUPBY", func(t *testing.T) {
		query := "SELECT * FROM t\nGROUP BY\n  a -- :if $1"
		sql, args := dbpostgres.DynamicSQL(query, []any{false})
		assertSQL(t, sql, "SELECT * FROM t")
		if len(args) != 0 {
			t.Errorf("args len: got %d, want 0", len(args))
		}
	})

	t.Run("OrphanedHAVING", func(t *testing.T) {
		query := "SELECT * FROM t\nGROUP BY a\nHAVING\n  count(*) > $1 -- :if $1"
		var c *int
		sql, args := dbpostgres.DynamicSQL(query, []any{c})
		assertSQL(t, sql, "SELECT * FROM t\nGROUP BY a")
		if len(args) != 0 {
			t.Errorf("args len: got %d, want 0", len(args))
		}
	})
}
