package golang

import (
	"slices"
	"testing"
)

func TestDynParenDeltas(t *testing.T) {
	cases := []struct {
		name, engine, sql string
		want              []int
	}{
		{"continued_escape_string", "postgresql", "EXISTS (\n name = E'foo'\n 'it\\'s (' || $1\n)", []int{1, 0, 0, -1}},
		{"continued_escape_after_line_comment", "postgresql", "E'foo' -- comment\n 'it\\'s (' AND (\n)", []int{0, 1, -1}},
		{"nested_sql", "postgresql", "EXISTS (\n AND (x = $1)\n)", []int{1, 0, -1}},
		{"multiline_string", "postgresql", "name = '(\n)\n' AND (x)", []int{0, 0, 0}},
		{"multiline_dollar_string", "postgresql", "name = $tag$(\n)\n$tag$ AND (", []int{0, 0, 1}},
		{"nested_multiline_comment", "postgresql", "(/* (\n /* ) */\n */)", []int{1, 0, -1}},
		{"doubled_identifier_quote", "postgresql", `"a""(" = $1 AND (`, []int{1}},
		{"pg_backslash_is_not_escape", "postgresql", `name = '\' AND (`, []int{1}},
		{"pg_escape_string", "postgresql", `name = E'it\'s (' AND (`, []int{1}},
		{"pg_dollar_in_identifier", "postgresql", "name$tag$ = $1 AND (", []int{1}},
		{"pg_unicode_dollar_tag", "postgresql", "name = $тег$($тег$ AND (", []int{1}},
		{"mysql_escape_string", "mysql", `name = 'it\'s (' AND (`, []int{1}},
		{"mysql_backticks", "mysql", "`a``(` = ? AND (", []int{1}},
		{"mysql_hash_comment", "mysql", "EXISTS ( # )\n)", []int{1, -1}},
		{"mysql_non_nested_comment", "mysql", "/* outer /* inner */ (", []int{1}},
		{"mysql_minus_minus", "mysql", "value--(1)", []int{0}},
		{"sqlite_brackets", "sqlite", "[a(] = ?1 AND (", []int{1}},
		{"sqlite_backslash_is_not_escape", "sqlite", `name = '\' AND (`, []int{1}},
		{"sqlite_backticks", "sqlite", "`a(` = ?1 AND (", []int{1}},
		{"sqlite_non_nested_comment", "sqlite", "/* outer /* inner */ (", []int{1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dynParenDeltas(tc.sql, tc.engine); !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
