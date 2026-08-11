package golang

import (
	"context"
	"strings"
	"testing"

	"github.com/sqlc-dev/plugin-sdk-go/plugin"
)

func TestGenerateDynamicFilter(t *testing.T) {
	cases := []dynamicFilterTest{
		{
			name:       "PostgreSQL",
			engine:     "postgresql",
			sqlPackage: "pgx/v5",
			query:      "SELECT id, name, status FROM items\nWHERE name = $1\n  AND status = $2 -- :if @status\nORDER BY\n  id ASC -- :if @id_asc\n  id DESC -- :if @id_desc",
			queryCall:  "rows, err := q.db.Query(ctx, dynQuery, dynArgs...)",
		},
		{
			name:       "SQLite",
			engine:     "sqlite",
			sqlPackage: "database/sql",
			query:      "SELECT id, name, status FROM items\nWHERE name = ?1\n  AND status = ?2 -- :if @status\nORDER BY\n  id ASC -- :if @id_asc\n  id DESC -- :if @id_desc",
			queryCall:  "rows, err := q.db.QueryContext(ctx, dynQuery, dynArgs...)",
			sqlite:     true,
		},
		{
			name:       "MySQL",
			engine:     "mysql",
			sqlPackage: "database/sql",
			sqlDriver:  "github.com/go-sql-driver/mysql",
			query:      "SELECT id, name, status FROM items\nWHERE name = ?\n  AND status = ? -- :if @status\nORDER BY\n  id ASC -- :if @id_asc\n  id DESC -- :if @id_desc",
			queryCall:  "rows, err := q.db.QueryContext(ctx, dynQuery, dynArgs...)",
			mysql:      true,
		},
		{
			// No sql_driver option: the engine alone must select question-mark
			// placeholders, or every dynamic query breaks at runtime.
			name:       "MySQLEngineOnly",
			engine:     "mysql",
			sqlPackage: "database/sql",
			query:      "SELECT id, name, status FROM items\nWHERE name = ?\n  AND status = ? -- :if @status\nORDER BY\n  id ASC -- :if @id_asc\n  id DESC -- :if @id_desc",
			queryCall:  "rows, err := q.db.QueryContext(ctx, dynQuery, dynArgs...)",
			mysql:      true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testGenerateDynamicFilter(t, tc)
		})
	}
}

type dynamicFilterTest struct {
	name, engine, sqlPackage, sqlDriver, query, queryCall string
	sqlite, mysql                                         bool
}

func testGenerateDynamicFilter(t *testing.T, tc dynamicFilterTest) {
	req := &plugin.GenerateRequest{
		SqlcVersion: "v1.0.0",
		Settings: &plugin.Settings{
			Engine: tc.engine,
		},
		Catalog: &plugin.Catalog{
			DefaultSchema: "public",
			Schemas: []*plugin.Schema{
				{
					Name: "public",
					Tables: []*plugin.Table{
						{
							Rel: &plugin.Identifier{Schema: "public", Name: "items"},
							Columns: []*plugin.Column{
								{Name: "id", NotNull: true, Type: &plugin.Identifier{Name: "bigint"}},
								{Name: "name", NotNull: true, Type: &plugin.Identifier{Name: "text"}},
								{Name: "status", NotNull: true, Type: &plugin.Identifier{Name: "text"}},
							},
						},
					},
				},
			},
		},
		Queries: []*plugin.Query{
			{
				Name:     "SearchItems",
				Cmd:      ":many",
				Filename: "query.sql",
				// Simulated SQL with :if annotations
				Text: tc.query,
				Params: []*plugin.Parameter{
					{Number: 1, Column: &plugin.Column{Name: "name", NotNull: true, Type: &plugin.Identifier{Name: "text"}}},
					{Number: 2, Column: &plugin.Column{Name: "status", NotNull: true, Type: &plugin.Identifier{Name: "text"}}},
				},
				Columns: []*plugin.Column{
					{Name: "id", NotNull: true, Type: &plugin.Identifier{Name: "bigint"}},
					{Name: "name", NotNull: true, Type: &plugin.Identifier{Name: "text"}},
					{Name: "status", NotNull: true, Type: &plugin.Identifier{Name: "text"}},
				},
			},
		},
		PluginOptions: []byte(`{"package":"testpkg","sql_package":"` + tc.sqlPackage + `","sql_driver":"` + tc.sqlDriver + `","emit_dynamic_filter":true}`),
		GlobalOptions: []byte(`{}`),
	}

	resp, err := Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	// Find the query file
	var queryFile, dynfilterFile string
	for _, f := range resp.Files {
		if f.Name == "query.sql.go" {
			queryFile = string(f.Contents)
		}
		if f.Name == "dynfilter.go" {
			dynfilterFile = string(f.Contents)
		}
	}

	if queryFile == "" {
		t.Fatal("query.sql.go not generated")
	}
	if dynfilterFile == "" {
		t.Fatal("dynfilter.go not generated")
	}

	// Verify params struct has pointer type for conditional param and bool fields for flags
	if !strings.Contains(queryFile, "*string") && !strings.Contains(queryFile, "Status *string") {
		t.Logf("query file:\n%s", queryFile)
		t.Error("expected *string pointer type for conditional 'status' param")
	}
	if !strings.Contains(queryFile, "IdAsc") {
		t.Logf("query file:\n%s", queryFile)
		t.Error("expected 'IdAsc' bool field in params struct")
	}
	if !strings.Contains(queryFile, "IdDesc") {
		t.Logf("query file:\n%s", queryFile)
		t.Error("expected 'IdDesc' bool field in params struct")
	}

	// Verify pre-compiled dynQuery var and Build call are in the generated method
	if !strings.Contains(queryFile, "dynCompile(") {
		t.Logf("query file:\n%s", queryFile)
		t.Error("expected dynCompile var declaration in generated query file")
	}
	if !strings.Contains(queryFile, ".Build(") {
		t.Logf("query file:\n%s", queryFile)
		t.Error("expected .Build( call in generated query method")
	}
	if !strings.Contains(queryFile, "dynQuery, dynArgs := _searchItemsDynQ.Build([]any{arg.Name, arg.Status, arg.IdAsc, arg.IdDesc})\n\t"+tc.queryCall) {
		t.Logf("query file:\n%s", queryFile)
		t.Error("expected dynamic-filter query and execution to be separate statements")
	}

	// Verify dynfilter.go contains the core functions
	if !strings.Contains(dynfilterFile, "func dynCompile(") {
		t.Logf("dynfilter file:\n%s", dynfilterFile)
		t.Error("expected dynCompile function in dynfilter.go")
	}
	if !strings.Contains(dynfilterFile, "func DynamicSQL(") {
		t.Logf("dynfilter file:\n%s", dynfilterFile)
		t.Error("expected DynamicSQL function in dynfilter.go")
	}
	if tc.sqlite && !strings.Contains(dynfilterFile, "const dynBracketIdentifiers = true") {
		t.Logf("dynfilter file:\n%s", dynfilterFile)
		t.Error("expected SQLite dynamic SQL to treat [name] as a quoted identifier")
	}
	if !tc.sqlite && !strings.Contains(dynfilterFile, "const dynBracketIdentifiers = false") {
		t.Logf("dynfilter file:\n%s", dynfilterFile)
		t.Error("expected non-SQLite dynamic SQL to treat [ ] as a subscript, not an identifier")
	}
	if tc.mysql && !strings.Contains(dynfilterFile, "const dynQuestionMarkPlaceholders = true") {
		t.Errorf("expected MySQL dynamic SQL to use question-mark placeholders:\n%s", dynfilterFile)
	}

	// Verify the SQL constant still has -- :if $N markers (used by dynCompile)
	if !strings.Contains(queryFile, "-- :if $") {
		t.Logf("query file:\n%s", queryFile)
		t.Error("expected '-- :if $N' markers in SQL constant")
	}

	t.Logf("Generated query.sql.go:\n%s", queryFile)
	t.Logf("Generated dynfilter.go:\n%s", dynfilterFile)
}

func TestGenerateDynamicFilter_ValidationError(t *testing.T) {
	req := &plugin.GenerateRequest{
		SqlcVersion: "v1.0.0",
		Settings:    &plugin.Settings{Engine: "postgresql"},
		Catalog: &plugin.Catalog{
			DefaultSchema: "public",
			Schemas:       []*plugin.Schema{{Name: "public"}},
		},
		Queries:       []*plugin.Query{},
		PluginOptions: []byte(`{"package":"testpkg","sql_package":"pgx/v5","emit_dynamic_filter":true,"emit_prepared_queries":true}`),
		GlobalOptions: []byte(`{}`),
	}
	_, err := Generate(context.Background(), req)
	if err == nil {
		t.Error("expected error for emit_dynamic_filter + emit_prepared_queries")
	}
	if !strings.Contains(err.Error(), "emit_dynamic_filter") {
		t.Errorf("expected error to mention emit_dynamic_filter, got: %v", err)
	}
}

// TestGenerateDynamicFilter_FlagOnlyParams verifies that a query with only
// flag-only params (no WHERE $N predicates) generates a params struct with
// only bool fields and calls DynamicSQL correctly.
func TestGenerateDynamicFilter_FlagOnlyParams(t *testing.T) {
	req := &plugin.GenerateRequest{
		SqlcVersion: "v1.0.0",
		Settings:    &plugin.Settings{Engine: "postgresql"},
		Catalog: &plugin.Catalog{
			DefaultSchema: "public",
			Schemas: []*plugin.Schema{{
				Name: "public",
				Tables: []*plugin.Table{{
					Rel:     &plugin.Identifier{Schema: "public", Name: "items"},
					Columns: []*plugin.Column{{Name: "id", NotNull: true, Type: &plugin.Identifier{Name: "bigint"}}},
				}},
			}},
		},
		Queries: []*plugin.Query{{
			Name:     "ListItems",
			Cmd:      ":many",
			Filename: "query.sql",
			Text:     "SELECT id FROM items\nORDER BY\n  id ASC -- :if @sort_asc\n  id DESC -- :if @sort_desc",
			Params:   []*plugin.Parameter{},
			Columns:  []*plugin.Column{{Name: "id", NotNull: true, Type: &plugin.Identifier{Name: "bigint"}}},
		}},
		PluginOptions: []byte(`{"package":"testpkg","sql_package":"pgx/v5","emit_dynamic_filter":true}`),
		GlobalOptions: []byte(`{}`),
	}

	resp, err := Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	var queryFile string
	for _, f := range resp.Files {
		if f.Name == "query.sql.go" {
			queryFile = string(f.Contents)
		}
	}
	if queryFile == "" {
		t.Fatal("query.sql.go not generated")
	}

	if !strings.Contains(queryFile, "SortAsc") {
		t.Errorf("expected SortAsc bool field, got:\n%s", queryFile)
	}
	if !strings.Contains(queryFile, "SortDesc") {
		t.Errorf("expected SortDesc bool field, got:\n%s", queryFile)
	}
	if !strings.Contains(queryFile, ".Build(") {
		t.Errorf("expected .Build( call, got:\n%s", queryFile)
	}
	t.Logf("Generated:\n%s", queryFile)
}

// TestGenerateDynamicFilter_BlockAnnotation verifies that a standalone
// block-style annotation (-- :if @flag on its own line) causes the next line
// to be skipped when the flag is false, and the marker appears in the SQL constant.
func TestGenerateDynamicFilter_BlockAnnotation(t *testing.T) {
	req := &plugin.GenerateRequest{
		SqlcVersion: "v1.0.0",
		Settings:    &plugin.Settings{Engine: "postgresql"},
		Catalog: &plugin.Catalog{
			DefaultSchema: "public",
			Schemas: []*plugin.Schema{{
				Name: "public",
				Tables: []*plugin.Table{{
					Rel: &plugin.Identifier{Schema: "public", Name: "items"},
					Columns: []*plugin.Column{
						{Name: "id", NotNull: true, Type: &plugin.Identifier{Name: "bigint"}},
						{Name: "status", Type: &plugin.Identifier{Name: "text"}},
					},
				}},
			}},
		},
		Queries: []*plugin.Query{{
			Name:     "GetItems",
			Cmd:      ":many",
			Filename: "query.sql",
			Text:     "SELECT id, status FROM items\nWHERE id = $1\n-- :if @filter_status\n  AND status = $2",
			Params: []*plugin.Parameter{
				{Number: 1, Column: &plugin.Column{Name: "id", NotNull: true, Type: &plugin.Identifier{Name: "bigint"}}},
				{Number: 2, Column: &plugin.Column{Name: "status", Type: &plugin.Identifier{Name: "text"}}},
			},
			Columns: []*plugin.Column{
				{Name: "id", NotNull: true, Type: &plugin.Identifier{Name: "bigint"}},
				{Name: "status", Type: &plugin.Identifier{Name: "text"}},
			},
		}},
		PluginOptions: []byte(`{"package":"testpkg","sql_package":"pgx/v5","emit_dynamic_filter":true}`),
		GlobalOptions: []byte(`{}`),
	}

	resp, err := Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	var queryFile string
	for _, f := range resp.Files {
		if f.Name == "query.sql.go" {
			queryFile = string(f.Contents)
		}
	}
	if queryFile == "" {
		t.Fatal("query.sql.go not generated")
	}

	// The SQL constant should have a standalone :if marker for the block annotation.
	if !strings.Contains(queryFile, "-- :if $") {
		t.Errorf("expected :if $N marker in SQL constant, got:\n%s", queryFile)
	}
	// Verify the new pre-compiled approach is used.
	if !strings.Contains(queryFile, "dynCompile(") {
		t.Errorf("expected dynCompile var declaration, got:\n%s", queryFile)
	}
	// filter_status is a flag-only param → bool field, not a pointer.
	if !strings.Contains(queryFile, "FilterStatus") {
		t.Errorf("expected FilterStatus bool field, got:\n%s", queryFile)
	}
	// status is NOT conditional (the block flag controls skipping, not the param itself),
	// so it should remain a plain type, not a pointer.
	if strings.Contains(queryFile, "*pgtype.Text") || strings.Contains(queryFile, "Status *") {
		t.Errorf("status should NOT be a pointer in block-annotation mode, got:\n%s", queryFile)
	}
	t.Logf("Generated:\n%s", queryFile)
}

// TestGenerateDynamicFilter_ArrayParam verifies that a conditional param whose
// Go type is a slice ([]T) is NOT wrapped in a pointer (*[]T). Slices are
// already nil-able, so nil means "skip this condition" without the extra level
// of indirection.
func TestGenerateDynamicFilter_ArrayParam(t *testing.T) {
	req := &plugin.GenerateRequest{
		SqlcVersion: "v1.0.0",
		Settings:    &plugin.Settings{Engine: "postgresql"},
		Catalog: &plugin.Catalog{
			DefaultSchema: "public",
			Schemas: []*plugin.Schema{{
				Name: "public",
				Tables: []*plugin.Table{{
					Rel: &plugin.Identifier{Schema: "public", Name: "items"},
					Columns: []*plugin.Column{
						{Name: "id", NotNull: true, Type: &plugin.Identifier{Name: "bigint"}},
						{Name: "name", NotNull: true, Type: &plugin.Identifier{Name: "text"}},
					},
				}},
			}},
		},
		Queries: []*plugin.Query{{
			Name:     "SearchItems",
			Cmd:      ":many",
			Filename: "query.sql",
			Text:     "SELECT id, name FROM items\nWHERE name = $1\n  AND id = ANY($2) -- :if @ids",
			Params: []*plugin.Parameter{
				{Number: 1, Column: &plugin.Column{Name: "name", NotNull: true, Type: &plugin.Identifier{Name: "text"}}},
				{Number: 2, Column: &plugin.Column{Name: "ids", IsArray: true, ArrayDims: 1, NotNull: true, Type: &plugin.Identifier{Name: "bigint"}}},
			},
			Columns: []*plugin.Column{
				{Name: "id", NotNull: true, Type: &plugin.Identifier{Name: "bigint"}},
				{Name: "name", NotNull: true, Type: &plugin.Identifier{Name: "text"}},
			},
		}},
		PluginOptions: []byte(`{"package":"testpkg","sql_package":"pgx/v5","emit_dynamic_filter":true}`),
		GlobalOptions: []byte(`{}`),
	}

	resp, err := Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	var queryFile string
	for _, f := range resp.Files {
		if f.Name == "query.sql.go" {
			queryFile = string(f.Contents)
		}
	}
	if queryFile == "" {
		t.Fatal("query.sql.go not generated")
	}

	// The slice type must NOT be wrapped in a pointer.
	if strings.Contains(queryFile, "*[]int64") {
		t.Logf("query file:\n%s", queryFile)
		t.Error("array param should not be wrapped in a pointer: got *[]int64, want []int64")
	}
	// The field must still be present as a plain slice.
	if !strings.Contains(queryFile, "[]int64") {
		t.Logf("query file:\n%s", queryFile)
		t.Error("expected []int64 field for array param")
	}
	t.Logf("Generated:\n%s", queryFile)
}

// TestGenerateDynamicFilter_ArrayParam_Single verifies the same nil-slice logic
// for a query with a single conditional array param (no struct wrapping).
func TestGenerateDynamicFilter_ArrayParam_Single(t *testing.T) {
	req := &plugin.GenerateRequest{
		SqlcVersion: "v1.0.0",
		Settings:    &plugin.Settings{Engine: "postgresql"},
		Catalog: &plugin.Catalog{
			DefaultSchema: "public",
			Schemas: []*plugin.Schema{{
				Name: "public",
				Tables: []*plugin.Table{{
					Rel:     &plugin.Identifier{Schema: "public", Name: "items"},
					Columns: []*plugin.Column{{Name: "id", NotNull: true, Type: &plugin.Identifier{Name: "bigint"}}},
				}},
			}},
		},
		Queries: []*plugin.Query{{
			Name:     "FilterByIDs",
			Cmd:      ":many",
			Filename: "query.sql",
			Text:     "SELECT id FROM items\nWHERE id = ANY($1) -- :if @ids",
			Params: []*plugin.Parameter{
				{Number: 1, Column: &plugin.Column{Name: "ids", IsArray: true, ArrayDims: 1, NotNull: true, Type: &plugin.Identifier{Name: "bigint"}}},
			},
			Columns: []*plugin.Column{
				{Name: "id", NotNull: true, Type: &plugin.Identifier{Name: "bigint"}},
			},
		}},
		PluginOptions: []byte(`{"package":"testpkg","sql_package":"pgx/v5","emit_dynamic_filter":true}`),
		GlobalOptions: []byte(`{}`),
	}

	resp, err := Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	var queryFile string
	for _, f := range resp.Files {
		if f.Name == "query.sql.go" {
			queryFile = string(f.Contents)
		}
	}
	if queryFile == "" {
		t.Fatal("query.sql.go not generated")
	}

	if strings.Contains(queryFile, "*[]int64") {
		t.Logf("query file:\n%s", queryFile)
		t.Error("single array param should not be wrapped in a pointer: got *[]int64, want []int64")
	}
	if !strings.Contains(queryFile, "[]int64") {
		t.Logf("query file:\n%s", queryFile)
		t.Error("expected []int64 type for single array param")
	}
	t.Logf("Generated:\n%s", queryFile)
}

func TestGenerateDynamicFilter_SqlcSlice(t *testing.T) {
	req := &plugin.GenerateRequest{
		SqlcVersion: "v1.0.0",
		Settings:    &plugin.Settings{Engine: "sqlite"},
		Catalog: &plugin.Catalog{DefaultSchema: "main", Schemas: []*plugin.Schema{{Name: "main", Tables: []*plugin.Table{{
			Rel: &plugin.Identifier{Schema: "main", Name: "items"},
			Columns: []*plugin.Column{
				{Name: "id", NotNull: true, Type: &plugin.Identifier{Name: "bigint"}},
				{Name: "name", NotNull: true, Type: &plugin.Identifier{Name: "text"}},
			},
		}}}}},
		Queries: []*plugin.Query{{
			Name:     "SearchItems",
			Cmd:      ":many",
			Filename: "query.sql",
			Text:     "SELECT id, name FROM items\nWHERE name = ?1\n  AND id IN (/*SLICE:team-ids*/?) -- :if @name",
			Params: []*plugin.Parameter{
				{Number: 1, Column: &plugin.Column{Name: "name", NotNull: true, Type: &plugin.Identifier{Name: "text"}}},
				{Number: 2, Column: &plugin.Column{Name: "team-ids", IsSqlcSlice: true, NotNull: true, Type: &plugin.Identifier{Name: "bigint"}}},
			},
			Columns: []*plugin.Column{
				{Name: "id", NotNull: true, Type: &plugin.Identifier{Name: "bigint"}},
				{Name: "name", NotNull: true, Type: &plugin.Identifier{Name: "text"}},
			},
		}},
		PluginOptions: []byte(`{"package":"testpkg","sql_package":"database/sql","emit_dynamic_filter":true}`),
		GlobalOptions: []byte(`{}`),
	}

	resp, err := Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range resp.Files {
		if f.Name != "query.sql.go" {
			continue
		}
		output := string(f.Contents)
		if strings.Contains(output, `"strings"`) {
			t.Errorf("dynamic sqlc.slice query should not import strings:\n%s", output)
		}
		if !strings.Contains(output, "/*SLICE:team-ids*/?2) -- :if $1") {
			t.Errorf("dynamic sqlc.slice marker should retain its parameter number:\n%s", output)
		}
		return
	}
	t.Fatal("query.sql.go not generated")
}
