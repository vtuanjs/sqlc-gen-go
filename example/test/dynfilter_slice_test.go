package db

import (
	"slices"
	"testing"

	"example/dbpostgres"
	"example/dbmysql"
	"example/dbsqlite"
)

func TestDynamicSQLSlices(t *testing.T) {
	t.Run("PostgreSQLSqlcSlice/Repeated", func(t *testing.T) {
		query := "SELECT * FROM t WHERE id IN (/*SLICE:ids*/$1) OR parent_id IN (/*SLICE:ids*/$1)"
		gotQuery, gotArgs := dbpostgres.DynamicSQL(query, []any{[]int64{7, 9}})

		assertSQL(t, gotQuery, "SELECT * FROM t WHERE id IN ($1,$2) OR parent_id IN ($1,$2)")
		if !slices.Equal(gotArgs, []any{int64(7), int64(9)}) {
			t.Errorf("args: got %v, want [7 9]", gotArgs)
		}
	})

	t.Run("SQLiteSqlcSlice/MissingArgument", func(t *testing.T) {
		query := "SELECT * FROM t WHERE id IN (/*SLICE:ids*/?2) -- :if $1"
		gotQuery, gotArgs := dbsqlite.DynamicSQL(query, []any{true})

		assertSQL(t, gotQuery, "SELECT * FROM t WHERE id IN (NULL)")
		if len(gotArgs) != 0 {
			t.Errorf("args: got %v, want none", gotArgs)
		}

		gotQuery, gotArgs = dbsqlite.CompileDynSQL(query).Build([]any{true})
		assertSQL(t, gotQuery, "SELECT * FROM t WHERE id IN (NULL)")
		if len(gotArgs) != 0 {
			t.Errorf("compiled args: got %v, want none", gotArgs)
		}
	})

	t.Run("PostgreSQLSqlcSlice/RepeatedEmpty", func(t *testing.T) {
		// Every occurrence of an empty slice must render NULL — replaying the
		// cached expansion as zero placeholders would emit invalid "IN ()".
		query := "SELECT * FROM t WHERE id IN (/*SLICE:ids*/$1) OR parent_id IN (/*SLICE:ids*/$1)"
		gotQuery, gotArgs := dbpostgres.DynamicSQL(query, []any{[]int64{}})

		assertSQL(t, gotQuery, "SELECT * FROM t WHERE id IN (NULL) OR parent_id IN (NULL)")
		if len(gotArgs) != 0 {
			t.Errorf("args: got %v, want none", gotArgs)
		}
	})

	t.Run("PostgreSQLSqlcSlice/SliceThenScalarSameParam", func(t *testing.T) {
		// A scalar reference to a param whose slice expansion rendered NULL
		// must bind normally instead of panicking on the cached nil entry.
		query := "SELECT * FROM t WHERE id IN (/*SLICE:ids*/$1) AND backup = $1"
		gotQuery, gotArgs := dbpostgres.DynamicSQL(query, []any{[]int64{}})

		assertSQL(t, gotQuery, "SELECT * FROM t WHERE id IN (NULL) AND backup = $1")
		if len(gotArgs) != 1 {
			t.Errorf("args len: got %d, want 1", len(gotArgs))
		}
	})

	t.Run("TrailingSliceCommentNoPlaceholder", func(t *testing.T) {
		// "/*SLICE:...*/" text with no placeholder after it in the segment must
		// be kept as ordinary text, not indexed as a slice marker (panic).
		query := "SELECT * FROM t\nWHERE a = $1 -- :if $1\n  AND b = 2 /*SLICE:foo*/"
		gotQuery, gotArgs := dbpostgres.DynamicSQL(query, []any{"x"})

		assertSQL(t, gotQuery, "SELECT * FROM t\nWHERE a = $1\n  AND b = 2 /*SLICE:foo*/")
		if !slices.Equal(gotArgs, []any{"x"}) {
			t.Errorf("args: got %v, want [x]", gotArgs)
		}
	})

	t.Run("SliceCommentBeforeScalarArg", func(t *testing.T) {
		// A slice-shaped comment in front of a placeholder whose arg is not a
		// slice binds as an ordinary scalar; the comment text is preserved.
		query := "SELECT * FROM t WHERE note = /*SLICE:legacy*/$1"
		gotQuery, gotArgs := dbpostgres.DynamicSQL(query, []any{"hello"})

		assertSQL(t, gotQuery, "SELECT * FROM t WHERE note = /*SLICE:legacy*/$1")
		if !slices.Equal(gotArgs, []any{"hello"}) {
			t.Errorf("args: got %v, want [hello]", gotArgs)
		}
	})

	t.Run("MySQLSqlcSlice/ActiveCondition", func(t *testing.T) {
		query := "SELECT * FROM t WHERE kind = ? AND id IN (/*SLICE:ids*/?2) -- :if $2"
		gotQuery, gotArgs := dbmysql.DynamicSQL(query, []any{"admin", []int64{7, 9}})

		assertSQL(t, gotQuery, "SELECT * FROM t WHERE kind = ? AND id IN (?,?)")
		if !slices.Equal(gotArgs, []any{"admin", int64(7), int64(9)}) {
			t.Errorf("args: got %v, want [admin 7 9]", gotArgs)
		}
	})
}
