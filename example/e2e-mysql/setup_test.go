package e2emysql

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"example/dbmysql"

	_ "github.com/go-sql-driver/mysql"
)

// parseTime=true so TIMESTAMP columns scan into time.Time.
const dsn = "root:mysql@tcp(localhost:6603)/sqlc-test?parseTime=true&multiStatements=true"

var (
	migrateOnce sync.Once
	migrateErr  error
)

// newDB connects to mysql, applies the schema once per test binary, and closes
// the connection when the test finishes.
func newDB(t testing.TB) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	var pingErr error
	connected := false
	for range 10 {
		if pingErr = db.Ping(); pingErr == nil {
			connected = true
			break
		}
		time.Sleep(time.Second)
	}
	if !connected {
		t.Fatalf("connect to mysql: %v", pingErr)
	}

	migrateOnce.Do(func() { migrateErr = migrate(db) })
	if migrateErr != nil {
		t.Fatalf("migrate: %v", migrateErr)
	}
	return db
}

func migrate(db *sql.DB) error {
	ctx := context.Background()
	// orders references users, so drop it first.
	for _, table := range []string{"orders", "products", "users", "filter_items"} {
		if _, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS "+table); err != nil {
			return err
		}
	}
	schema, err := os.ReadFile("../mysql/schema.sql")
	if err != nil {
		return err
	}
	for stmt := range strings.SplitSeq(string(schema), ";") {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// insertUser inserts a user and deletes it (plus its orders) after the test.
func insertUser(t testing.TB, db *sql.DB, name, email string, phone *string) dbmysql.User {
	t.Helper()
	ctx := context.Background()

	res, err := db.ExecContext(ctx,
		"INSERT INTO users (name, email, phone) VALUES (?, ?, ?)", name, email, phone)
	if err != nil {
		t.Fatalf("insertUser: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("insertUser id: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM orders WHERE user_id = ?", id)
		db.Exec("DELETE FROM users WHERE id = ?", id)
	})

	u, err := dbmysql.New(db).GetUser(ctx, id)
	if err != nil {
		t.Fatalf("insertUser read back: %v", err)
	}
	return u
}

// insertOrder inserts an order for a user and deletes it after the test.
func insertOrder(t testing.TB, db *sql.DB, userID int64, createdAt time.Time) int64 {
	t.Helper()

	res, err := db.Exec(
		"INSERT INTO orders (user_id, amount, created_at) VALUES (?, 1.00, ?)", userID, createdAt)
	if err != nil {
		t.Fatalf("insertOrder: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("insertOrder id: %v", err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM orders WHERE id = ?", id) })
	return id
}

func strPtr(v string) *string { return &v }

// nullStr builds the *sql.NullString a `-- :if` annotated nullable column
// parameter expects: non-nil pointer = filter active.
func nullStr(v string) *sql.NullString {
	return &sql.NullString{String: v, Valid: true}
}
