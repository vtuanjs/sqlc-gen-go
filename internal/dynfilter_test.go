package golang

import (
	"strings"
	"testing"

	"github.com/sqlc-dev/plugin-sdk-go/plugin"
)

func makeParam(name string, number int32) *plugin.Parameter {
	return &plugin.Parameter{
		Number: number,
		Column: &plugin.Column{Name: name},
	}
}

func TestParseDynFilter_NoAnnotations(t *testing.T) {
	sql := `SELECT * FROM t WHERE a = $1 AND b = $2`
	info, err := ParseDynFilter(sql, []*plugin.Parameter{
		makeParam("a", 1),
		makeParam("b", 2),
	})
	if err != nil {
		t.Fatal(err)
	}
	if info != nil {
		t.Error("expected nil for SQL without :if annotations")
	}
}

func TestParseDynFilter_InlineAnnotation(t *testing.T) {
	// params: a=$1 (required), b=$2 (conditional), c=$3 (conditional)
	sql := "SELECT * FROM t\nWHERE a = $1\n  AND b = $2 -- :if @b\n  AND c = $3 -- :if @c"
	params := []*plugin.Parameter{
		makeParam("a", 1),
		makeParam("b", 2),
		makeParam("c", 3),
	}
	info, err := ParseDynFilter(sql, params)
	if err != nil {
		t.Fatal(err)
	}
	if info == nil {
		t.Fatal("expected non-nil DynFilterInfo")
	}

	// Should have 2 conditional param numbers ($2 and $3)
	if len(info.ConditionalParamNumbers) != 2 {
		t.Errorf("expected 2 conditional param numbers, got %d: %v", len(info.ConditionalParamNumbers), info.ConditionalParamNumbers)
	}

	// Should have no flag params
	if len(info.FlagParams) != 0 {
		t.Errorf("expected 0 flag params, got %d", len(info.FlagParams))
	}

	// OrderedArgNames = all SQL params in $N order: ["a", "b", "c"]
	if len(info.OrderedArgNames) != 3 {
		t.Fatalf("expected 3 ordered arg names (all SQL params), got %d: %v", len(info.OrderedArgNames), info.OrderedArgNames)
	}
	if info.OrderedArgNames[0] != "a" || info.OrderedArgNames[1] != "b" || info.OrderedArgNames[2] != "c" {
		t.Errorf("unexpected ordered arg names: %v", info.OrderedArgNames)
	}

	// b=$2 → :if $2, c=$3 → :if $3
	if !strings.Contains(info.AnnotatedSQL, "-- :if $2") {
		t.Errorf("expected :if $2 for b ($2) in annotated SQL, got:\n%s", info.AnnotatedSQL)
	}
	if !strings.Contains(info.AnnotatedSQL, "-- :if $3") {
		t.Errorf("expected :if $3 for c ($3) in annotated SQL, got:\n%s", info.AnnotatedSQL)
	}
	if strings.Contains(info.AnnotatedSQL, "-- :if @") {
		t.Errorf("original :if @name annotations should be replaced:\n%s", info.AnnotatedSQL)
	}
}

func TestParseDynFilter_FlagOnlyParam(t *testing.T) {
	// a=$1 (required), id_asc and id_desc are flag-only (ORDER BY flags)
	sql := "SELECT * FROM t\nWHERE a = $1\nORDER BY\n  id ASC -- :if @id_asc\n  id DESC -- :if @id_desc"
	params := []*plugin.Parameter{
		makeParam("a", 1),
	}
	info, err := ParseDynFilter(sql, params)
	if err != nil {
		t.Fatal(err)
	}
	if info == nil {
		t.Fatal("expected non-nil DynFilterInfo")
	}

	// No conditional SQL params
	if len(info.ConditionalParamNumbers) != 0 {
		t.Errorf("expected 0 conditional SQL params, got %d: %v", len(info.ConditionalParamNumbers), info.ConditionalParamNumbers)
	}

	// Two flag params
	if len(info.FlagParams) != 2 {
		t.Fatalf("expected 2 flag params, got %d", len(info.FlagParams))
	}
	if info.FlagParams[0].Name != "id_asc" || info.FlagParams[0].GoName != "IdAsc" {
		t.Errorf("unexpected flag param 0: %+v", info.FlagParams[0])
	}
	if info.FlagParams[1].Name != "id_desc" {
		t.Errorf("unexpected flag param 1: %+v", info.FlagParams[1])
	}

	// OrderedArgNames = ["a", "id_asc", "id_desc"]
	// a is at index 0 (SQL param), flags at 1 and 2
	if len(info.OrderedArgNames) != 3 {
		t.Fatalf("expected 3 ordered arg names, got %d: %v", len(info.OrderedArgNames), info.OrderedArgNames)
	}
	if info.OrderedArgNames[0] != "a" || info.OrderedArgNames[1] != "id_asc" || info.OrderedArgNames[2] != "id_desc" {
		t.Errorf("unexpected ordered arg names: %v", info.OrderedArgNames)
	}

	// id_asc: idx=len(params)+0=1 → $2; id_desc: idx=len(params)+1=2 → $3
	if !strings.Contains(info.AnnotatedSQL, "-- :if $2") {
		t.Errorf("expected :if $2 for id_asc:\n%s", info.AnnotatedSQL)
	}
	if !strings.Contains(info.AnnotatedSQL, "-- :if $3") {
		t.Errorf("expected :if $3 for id_desc:\n%s", info.AnnotatedSQL)
	}
}

func TestParseDynFilter_MixedParams(t *testing.T) {
	// a=$1 (required), b=$2 (conditional SQL param), id_asc (flag-only)
	sql := "SELECT * FROM t\nWHERE a = $1\n  AND b = $2 -- :if @b\nORDER BY\n  id ASC -- :if @id_asc"
	params := []*plugin.Parameter{
		makeParam("a", 1),
		makeParam("b", 2),
	}
	info, err := ParseDynFilter(sql, params)
	if err != nil {
		t.Fatal(err)
	}
	if info == nil {
		t.Fatal("expected non-nil DynFilterInfo")
	}

	// 1 conditional SQL param ($2)
	if len(info.ConditionalParamNumbers) != 1 || info.ConditionalParamNumbers[0] != 2 {
		t.Errorf("expected [2], got %v", info.ConditionalParamNumbers)
	}

	// 1 flag param
	if len(info.FlagParams) != 1 || info.FlagParams[0].Name != "id_asc" {
		t.Errorf("unexpected flag params: %v", info.FlagParams)
	}

	// OrderedArgNames = ["a", "b", "id_asc"]
	if len(info.OrderedArgNames) != 3 {
		t.Fatalf("expected 3 ordered arg names, got %d: %v", len(info.OrderedArgNames), info.OrderedArgNames)
	}
	if info.OrderedArgNames[0] != "a" || info.OrderedArgNames[1] != "b" || info.OrderedArgNames[2] != "id_asc" {
		t.Errorf("unexpected order: %v", info.OrderedArgNames)
	}

	// b=$2 → :if $2; id_asc: idx=len(params)+0=2 → :if $3
	if !strings.Contains(info.AnnotatedSQL, "-- :if $2") {
		t.Errorf("expected :if $2 for b:\n%s", info.AnnotatedSQL)
	}
	if !strings.Contains(info.AnnotatedSQL, "-- :if $3") {
		t.Errorf("expected :if $3 for id_asc:\n%s", info.AnnotatedSQL)
	}
}

func TestParseDynFilter_InlineBlockPropagation(t *testing.T) {
	// Inline annotation on a line that opens a multi-line paren block.
	// All lines in the block should be annotated with the same :if $N.
	sql := "SELECT * FROM t\nWHERE a = $1\n  AND EXISTS ( -- :if @has_orders\n    SELECT 1 FROM orders\n    WHERE orders.user_id = t.id\n  )"
	params := []*plugin.Parameter{
		makeParam("a", 1),
		makeParam("has_orders", 2),
	}
	info, err := ParseDynFilter(sql, params)
	if err != nil {
		t.Fatal(err)
	}
	if info == nil {
		t.Fatal("expected non-nil DynFilterInfo")
	}

	lines := strings.Split(info.AnnotatedSQL, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Every line that is part of the EXISTS block should carry the annotation.
		isBlockLine := strings.Contains(trimmed, "EXISTS") ||
			strings.Contains(trimmed, "SELECT 1") ||
			strings.Contains(trimmed, "orders.user_id") ||
			trimmed == ")"
		if isBlockLine && !strings.Contains(line, "-- :if $2") {
			t.Errorf("expected '-- :if $2' on block line %q, got:\n%s", line, info.AnnotatedSQL)
		}
	}
	t.Logf("AnnotatedSQL:\n%s", info.AnnotatedSQL)
}

func TestParseDynFilter_MultiParamAnnotation(t *testing.T) {
	// Line is included only when BOTH email AND phone are active.
	sql := "SELECT * FROM t\nWHERE a = $1\n  AND (email = $2 OR phone = $3) -- :if @email @phone"
	params := []*plugin.Parameter{
		makeParam("a", 1),
		makeParam("email", 2),
		makeParam("phone", 3),
	}
	info, err := ParseDynFilter(sql, params)
	if err != nil {
		t.Fatal(err)
	}
	if info == nil {
		t.Fatal("expected non-nil DynFilterInfo")
	}

	// Both email=$2 and phone=$3 should appear as conditions on the annotated line.
	var combinedLine string
	for _, line := range strings.Split(info.AnnotatedSQL, "\n") {
		if strings.Contains(line, "email = $2 OR phone") {
			combinedLine = line
		}
	}
	if combinedLine == "" {
		t.Fatalf("could not find combined condition line in:\n%s", info.AnnotatedSQL)
	}
	if !strings.Contains(combinedLine, "-- :if $2") {
		t.Errorf("expected '-- :if $2' on combined line, got: %q", combinedLine)
	}
	if !strings.Contains(combinedLine, "-- :if $3") {
		t.Errorf("expected '-- :if $3' on combined line, got: %q", combinedLine)
	}
	// Both should be conditional param numbers.
	if len(info.ConditionalParamNumbers) != 2 {
		t.Errorf("expected 2 conditional params, got %v", info.ConditionalParamNumbers)
	}
	t.Logf("AnnotatedSQL:\n%s", info.AnnotatedSQL)
}

func TestParseDynFilter_BlockAnnotation(t *testing.T) {
	sql := "SELECT * FROM t\nWHERE a = $1\n-- :if @b\n  AND b = $2"
	params := []*plugin.Parameter{
		makeParam("a", 1),
		makeParam("b", 2),
	}
	info, err := ParseDynFilter(sql, params)
	if err != nil {
		t.Fatal(err)
	}
	if info == nil {
		t.Fatal("expected non-nil DynFilterInfo")
	}

	// b=$2 → standalone block marker "-- :if $2"
	lines := strings.Split(info.AnnotatedSQL, "\n")
	var hasBlockMarker bool
	for _, line := range lines {
		if strings.TrimSpace(line) == "-- :if $2" {
			hasBlockMarker = true
		}
	}
	if !hasBlockMarker {
		t.Errorf("expected standalone :if $2 marker in annotated SQL:\n%s", info.AnnotatedSQL)
	}
}

func TestParseDynFilter_TopLevelBlockAnnotation(t *testing.T) {
	// Top-level annotation (standalone -- :if @flag) followed by a line that
	// opens a multi-line paren block. All interior lines should carry the
	// same condition so the block is skipped atomically.
	sql := "SELECT * FROM t\nWHERE a = $1\n-- :if @has_orders\n  AND EXISTS (\n    SELECT 1 FROM orders\n    WHERE orders.user_id = t.id\n  )"
	params := []*plugin.Parameter{
		makeParam("a", 1),
		makeParam("has_orders", 2),
	}
	info, err := ParseDynFilter(sql, params)
	if err != nil {
		t.Fatal(err)
	}
	if info == nil {
		t.Fatal("expected non-nil DynFilterInfo")
	}

	lines := strings.Split(info.AnnotatedSQL, "\n")
	// The standalone :if $2 marker must still be present for dynCompile.
	var hasBlockMarker bool
	for _, line := range lines {
		if strings.TrimSpace(line) == "-- :if $2" {
			hasBlockMarker = true
		}
	}
	if !hasBlockMarker {
		t.Errorf("expected standalone :if $2 marker in annotated SQL:\n%s", info.AnnotatedSQL)
	}
	// Every line inside the EXISTS block must also carry an inline annotation.
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		isBlockLine := strings.Contains(trimmed, "EXISTS") ||
			strings.Contains(trimmed, "SELECT 1") ||
			strings.Contains(trimmed, "orders.user_id") ||
			trimmed == ")"
		if isBlockLine && !strings.Contains(line, "-- :if $2") {
			t.Errorf("expected '-- :if $2' on block line %q, got:\n%s", line, info.AnnotatedSQL)
		}
	}
	t.Logf("AnnotatedSQL:\n%s", info.AnnotatedSQL)
}

func TestParseDynFilter_DollarPrefixAnnotation(t *testing.T) {
	// The :if annotation accepts $name as well as @name.
	sql := "SELECT * FROM t\nWHERE a = $1\n  AND b = $2 -- :if $b"
	params := []*plugin.Parameter{
		makeParam("a", 1),
		makeParam("b", 2),
	}
	info, err := ParseDynFilter(sql, params)
	if err != nil {
		t.Fatal(err)
	}
	if info == nil {
		t.Fatal("expected non-nil DynFilterInfo for $-prefixed annotation")
	}
	if len(info.ConditionalParamNumbers) != 1 || info.ConditionalParamNumbers[0] != 2 {
		t.Errorf("expected conditional param $2, got %v", info.ConditionalParamNumbers)
	}
}

func TestParseDynFilter_DuplicateParamInMultipleAnnotations(t *testing.T) {
	// The same param referenced in two different :if annotations should be deduped.
	sql := "SELECT * FROM t\nWHERE a = $1\n  AND b = $2 -- :if @b\n  AND c = $3 -- :if @b @c"
	params := []*plugin.Parameter{
		makeParam("a", 1),
		makeParam("b", 2),
		makeParam("c", 3),
	}
	info, err := ParseDynFilter(sql, params)
	if err != nil {
		t.Fatal(err)
	}
	if info == nil {
		t.Fatal("expected non-nil DynFilterInfo")
	}
	// Both b and c are conditional, deduped.
	if len(info.ConditionalParamNumbers) != 2 {
		t.Errorf("expected 2 conditional params (b, c), got %v", info.ConditionalParamNumbers)
	}
	// OrderedArgNames should not have duplicates: a, b, c
	seen := map[string]int{}
	for _, name := range info.OrderedArgNames {
		seen[name]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Errorf("duplicate param %q in OrderedArgNames: %v", name, info.OrderedArgNames)
		}
	}
}

func TestParseDynFilter_EmptySQL(t *testing.T) {
	info, err := ParseDynFilter("", nil)
	if err != nil {
		t.Fatal(err)
	}
	if info != nil {
		t.Error("expected nil for empty SQL")
	}
}

func TestParseDynFilter_OnlyRequiredParams(t *testing.T) {
	// SQL has params but no :if annotations → nil result.
	sql := "SELECT * FROM t WHERE a = $1 AND b = $2"
	params := []*plugin.Parameter{
		makeParam("a", 1),
		makeParam("b", 2),
	}
	info, err := ParseDynFilter(sql, params)
	if err != nil {
		t.Fatal(err)
	}
	if info != nil {
		t.Error("expected nil when no :if annotations are present")
	}
}

func TestParseDynFilter_SqlcSlicePunctuatedName(t *testing.T) {
	info, err := ParseDynFilter(
		"SELECT * FROM t\nWHERE active = ?1\n  AND id IN (/*SLICE:team-ids*/?) -- :if @active",
		[]*plugin.Parameter{makeParam("active", 1), makeParam("team-ids", 2)},
	)
	if err != nil {
		t.Fatal(err)
	}
	if info == nil {
		t.Fatal("expected non-nil DynFilterInfo")
	}
	if !strings.Contains(info.AnnotatedSQL, "/*SLICE:team-ids*/?2") {
		t.Errorf("expected punctuated slice marker to be numbered:\n%s", info.AnnotatedSQL)
	}
}
