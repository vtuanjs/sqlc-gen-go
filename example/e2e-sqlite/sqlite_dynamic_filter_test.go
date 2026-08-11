package e2esqlite

import (
	"context"
	"os"
	"testing"

	"example/dbsqlite"
	sqlite3driver "github.com/ncruces/go-sqlite3/driver"
)

func TestDynamicFilter(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite3driver.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	schema, err := os.ReadFile("../sqlite/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, string(schema)); err != nil {
		t.Fatal(err)
	}
	for _, row := range [][3]string{
		{"required-a", "first", "required-c"},
		{"required-a", "second", "required-c"},
		{"required-a", "first", "other-c"},
	} {
		if _, err := db.ExecContext(ctx, "INSERT INTO filter_items (a, b, c) VALUES (?, ?, ?)", row[0], row[1], row[2]); err != nil {
			t.Fatal(err)
		}
	}

	q := dbsqlite.New(db)
	items, err := q.ListFilterItems(ctx, dbsqlite.ListFilterItemsParams{A: "required-a", C: "required-c"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Errorf("nil b: got %d rows, want 2", len(items))
	}

	b := "first"
	items, err = q.ListFilterItems(ctx, dbsqlite.ListFilterItemsParams{A: "required-a", B: &b, C: "required-c"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].B != b {
		t.Errorf("non-nil b: got %v, want only %q", items, b)
	}
}
