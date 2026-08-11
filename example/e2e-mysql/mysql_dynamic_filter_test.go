package e2emysql

import (
	"context"
	"database/sql"
	"os"
	"reflect"
	"testing"
	"time"

	"example/dbmysql"

	_ "github.com/go-sql-driver/mysql"
)

const dsn = "root:mysql@tcp(localhost:6603)/sqlc-test"

func newDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	var pingErr error
	for range 10 {
		if pingErr = db.Ping(); pingErr == nil {
			return db
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("connect to mysql: %v", pingErr)
	return nil
}

func TestDynamicFilter(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)

	if _, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS filter_items"); err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile("../mysql/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, string(schema)); err != nil {
		t.Fatal(err)
	}
	for id, kind := range map[int64]string{1: "widget", 2: "widget", 3: "gadget"} {
		if _, err := db.ExecContext(ctx, "INSERT INTO filter_items (id, kind) VALUES (?, ?)", id, kind); err != nil {
			t.Fatal(err)
		}
	}

	q := dbmysql.New(db)

	ids, err := q.SearchFilterItems(ctx, dbmysql.SearchFilterItemsParams{Kind: "widget"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Errorf("nil ids: got %v, want 2 rows", ids)
	}

	ids, err = q.SearchFilterItems(ctx, dbmysql.SearchFilterItemsParams{Kind: "widget", Ids: []int64{2, 3}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ids, []int64{2}) {
		t.Errorf("ids filter: got %v, want [2]", ids)
	}

	// empty non-nil slice → condition active → IN (NULL) → zero rows
	ids, err = q.SearchFilterItems(ctx, dbmysql.SearchFilterItemsParams{Kind: "widget", Ids: []int64{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Errorf("empty ids: got %v, want 0 rows (filter by empty set)", ids)
	}

	// NilableSlice(empty) → nil → clause skipped for callers who want
	// empty to mean "don't filter"
	ids, err = q.SearchFilterItems(ctx, dbmysql.SearchFilterItemsParams{Kind: "widget", Ids: dbmysql.NilableSlice([]int64{})})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Errorf("NilableSlice(empty) ids: got %v, want 2 rows (clause skipped)", ids)
	}
}
