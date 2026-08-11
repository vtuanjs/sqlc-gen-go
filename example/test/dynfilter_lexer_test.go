package db

import (
	"slices"
	"testing"

	"example/dbpostgres"
	"example/dbmysql"
	"example/dbsqlite"
)

// TestDynamicSQL_LexicalContext proves the dynamic-filter runtime only rewrites
// bind markers and interprets "-- :if $N" annotations that appear in SQL code —
// never inside string literals, quoted identifiers, or comments. Each case
// asserts both the final SQL and the final arguments.
//
// dbpostgres.DynamicSQL uses the PostgreSQL runtime ($N output, [ ] is a subscript);
// dbsqlite/dbmysql exercise the SQLite bracket and MySQL backtick identifiers.
func TestDynamicSQL_LexicalContext(t *testing.T) {
	t.Run("SingleQuotedMarkers", func(t *testing.T) {
		// '?1' and '$2' are string constants and must survive untouched; only the
		// real $1 outside the strings is a placeholder.
		query := "SELECT note FROM t WHERE note = '?1' OR tag = '$2' OR a = $1"
		sql, args := dbpostgres.DynamicSQL(query, []any{"x"})
		assertSQL(t, sql, "SELECT note FROM t WHERE note = '?1' OR tag = '$2' OR a = $1")
		if !slices.Equal(args, []any{"x"}) {
			t.Errorf("args: got %v, want [x]", args)
		}
	})

	t.Run("EscapedQuote", func(t *testing.T) {
		// The doubled '' keeps the literal open, so ?1 stays inside the string.
		query := "SELECT note FROM t WHERE note = 'it''s ?1' AND a = $1"
		sql, args := dbpostgres.DynamicSQL(query, []any{"x"})
		assertSQL(t, sql, "SELECT note FROM t WHERE note = 'it''s ?1' AND a = $1")
		if !slices.Equal(args, []any{"x"}) {
			t.Errorf("args: got %v, want [x]", args)
		}
	})

	t.Run("LineComment", func(t *testing.T) {
		// A trailing "-- ..." comment (not a :if annotation) is preserved, and its
		// ?2/$3 are not counted as arguments.
		query := "SELECT a FROM t\nWHERE a = $1 -- drop ?2 and $3"
		sql, args := dbpostgres.DynamicSQL(query, []any{"x"})
		assertSQL(t, sql, "SELECT a FROM t\nWHERE a = $1 -- drop ?2 and $3")
		if !slices.Equal(args, []any{"x"}) {
			t.Errorf("args: got %v, want [x]", args)
		}
	})

	t.Run("BlockComment", func(t *testing.T) {
		query := "SELECT /* $2 and ?3 */ a FROM t WHERE a = $1"
		sql, args := dbpostgres.DynamicSQL(query, []any{"x"})
		assertSQL(t, sql, "SELECT /* $2 and ?3 */ a FROM t WHERE a = $1")
		if !slices.Equal(args, []any{"x"}) {
			t.Errorf("args: got %v, want [x]", args)
		}
	})

	t.Run("DoubleQuotedIdentifier", func(t *testing.T) {
		// $2 is part of the quoted column name, not a placeholder.
		query := `SELECT "weird$2col" FROM t WHERE a = $1`
		sql, args := dbpostgres.DynamicSQL(query, []any{"x"})
		assertSQL(t, sql, `SELECT "weird$2col" FROM t WHERE a = $1`)
		if !slices.Equal(args, []any{"x"}) {
			t.Errorf("args: got %v, want [x]", args)
		}
	})

	t.Run("AnnotationInsideStringLiteral", func(t *testing.T) {
		// The "-- :if $1" is inside a string constant; it must NOT truncate the
		// line or make it conditional. A nil first arg would drop the line if the
		// annotation were (wrongly) honored.
		query := "SELECT a FROM t\nWHERE note = 'x -- :if $1 y' AND a = $1"
		sql, args := dbpostgres.DynamicSQL(query, []any{"x"})
		assertSQL(t, sql, "SELECT a FROM t\nWHERE note = 'x -- :if $1 y' AND a = $1")
		if !slices.Equal(args, []any{"x"}) {
			t.Errorf("args: got %v, want [x]", args)
		}
	})

	t.Run("PostgresBracketIsSubscript", func(t *testing.T) {
		// PostgreSQL [ ] is an array subscript, so $1 inside it is a real bind.
		query := "SELECT tags[$1] FROM t WHERE a = $2"
		sql, args := dbpostgres.DynamicSQL(query, []any{int64(3), "x"})
		assertSQL(t, sql, "SELECT tags[$1] FROM t WHERE a = $2")
		if !slices.Equal(args, []any{int64(3), "x"}) {
			t.Errorf("args: got %v, want [3 x]", args)
		}
	})

	t.Run("SQLiteBracketIdentifier", func(t *testing.T) {
		// SQLite [ ] quotes an identifier, so ?1/$2 inside brackets are literal
		// name characters; only the trailing ?1 is a bind marker (→ $1).
		query := "SELECT [a?1b], [x$2y] FROM t WHERE a = ?1"
		sql, args := dbsqlite.DynamicSQL(query, []any{"x"})
		assertSQL(t, sql, "SELECT [a?1b], [x$2y] FROM t WHERE a = $1")
		if !slices.Equal(args, []any{"x"}) {
			t.Errorf("args: got %v, want [x]", args)
		}
	})

	t.Run("MySQLBacktickIdentifier", func(t *testing.T) {
		// MySQL backtick-quoted identifier: the ? inside is a name character, not
		// a positional placeholder.
		query := "SELECT `weird?col` FROM t WHERE a = ?"
		sql, args := dbmysql.DynamicSQL(query, []any{"x"})
		assertSQL(t, sql, "SELECT `weird?col` FROM t WHERE a = ?")
		if !slices.Equal(args, []any{"x"}) {
			t.Errorf("args: got %v, want [x]", args)
		}
	})

	t.Run("MySQLBackslashEscapedQuote", func(t *testing.T) {
		// MySQL's default sql_mode escapes quotes with a backslash; the string
		// must not swallow the rest of the line, so the annotation after it is
		// honored and the ? placeholder is counted.
		query := "SELECT a FROM t\nWHERE cond = 1\n  AND name = 'O\\'Brien' AND status = ? -- :if $1"
		sql, args := dbmysql.DynamicSQL(query, []any{"x"})
		assertSQL(t, sql, "SELECT a FROM t\nWHERE cond = 1\n  AND name = 'O\\'Brien' AND status = ?")
		if !slices.Equal(args, []any{"x"}) {
			t.Errorf("args: got %v, want [x]", args)
		}

		sql, args = dbmysql.DynamicSQL(query, []any{nil})
		assertSQL(t, sql, "SELECT a FROM t\nWHERE cond = 1")
		if len(args) != 0 {
			t.Errorf("args: got %v, want none", args)
		}
	})

	t.Run("PostgresDollarQuotedString", func(t *testing.T) {
		// $N-looking text inside a dollar-quoted literal is string content: it
		// must survive renumbering untouched when an earlier placeholder drops.
		query := "SELECT a FROM t\nWHERE 1 = 1\n  AND b = $1 -- :if $1\n  AND note = $$costs $2 up$$ AND c = $2"
		sql, args := dbpostgres.DynamicSQL(query, []any{nil, "c"})
		assertSQL(t, sql, "SELECT a FROM t\nWHERE 1 = 1\n  AND note = $$costs $2 up$$ AND c = $1")
		if !slices.Equal(args, []any{"c"}) {
			t.Errorf("args: got %v, want [c]", args)
		}
	})

	t.Run("PostgresDollarTagAnnotationImmune", func(t *testing.T) {
		// A tagged dollar quote hides annotation-shaped text from the scanner.
		query := "SELECT $tag$fake -- :if $9$tag$ FROM t WHERE a = $1"
		sql, args := dbpostgres.DynamicSQL(query, []any{"x"})
		assertSQL(t, sql, "SELECT $tag$fake -- :if $9$tag$ FROM t WHERE a = $1")
		if !slices.Equal(args, []any{"x"}) {
			t.Errorf("args: got %v, want [x]", args)
		}
	})

	t.Run("PostgresEscapeString", func(t *testing.T) {
		// E'...' strings use backslash escapes: the \' stays inside the literal.
		query := "SELECT a FROM t WHERE note = E'it\\'s $2' AND a = $1"
		sql, args := dbpostgres.DynamicSQL(query, []any{"x"})
		assertSQL(t, sql, "SELECT a FROM t WHERE note = E'it\\'s $2' AND a = $1")
		if !slices.Equal(args, []any{"x"}) {
			t.Errorf("args: got %v, want [x]", args)
		}
	})

	t.Run("PostgresNestedBlockComment", func(t *testing.T) {
		// PostgreSQL block comments nest; text after an inner close is still
		// comment, so the annotation there must be ignored and nothing dropped.
		query := "SELECT a /* outer /* inner */ note -- :if $1 */ FROM t WHERE a = $1"
		sql, args := dbpostgres.DynamicSQL(query, []any{"x"})
		assertSQL(t, sql, "SELECT a /* outer /* inner */ note -- :if $1 */ FROM t WHERE a = $1")
		if !slices.Equal(args, []any{"x"}) {
			t.Errorf("args: got %v, want [x]", args)
		}
	})

	t.Run("MultiLineStringLiteral", func(t *testing.T) {
		// A string literal spanning lines: annotation-shaped text on the
		// continuation line is string content, never a real annotation.
		query := "SELECT * FROM t\nWHERE note = 'hello\nworld -- :if $1' AND active = $2"
		sql, args := dbpostgres.DynamicSQL(query, []any{nil, "act"})
		assertSQL(t, sql, "SELECT * FROM t\nWHERE note = 'hello\nworld -- :if $1' AND active = $1")
		if !slices.Equal(args, []any{"act"}) {
			t.Errorf("args: got %v, want [act]", args)
		}
	})

	t.Run("MultiLineStringFakeBlockAnnotation", func(t *testing.T) {
		// A continuation line reading exactly "-- :if $1" is string content and
		// must not become a block annotation that truncates the query.
		query := "SELECT * FROM t\nWHERE x = 'abc\n-- :if $1\ndef' AND y = $2"
		sql, args := dbpostgres.DynamicSQL(query, []any{nil, "world"})
		assertSQL(t, sql, "SELECT * FROM t\nWHERE x = 'abc\n-- :if $1\ndef' AND y = $1")
		if !slices.Equal(args, []any{"world"}) {
			t.Errorf("args: got %v, want [world]", args)
		}
	})
}
