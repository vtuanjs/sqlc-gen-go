package e2esqlite

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"example/dbsqlite"

	sqlite3driver "github.com/ncruces/go-sqlite3/driver"
)

// newDB opens a fresh in-memory database and applies the schema. Unlike the
// postgres/mysql suites each test gets its own database, so no cleanup between
// tests is needed.
func newDB(t testing.TB) *sql.DB {
	t.Helper()

	db, err := sqlite3driver.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	// :memory: is per-connection, so the pool must hold a single connection.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	schema, err := os.ReadFile("../sqlite/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(), string(schema)); err != nil {
		t.Fatal(err)
	}
	return db
}

func insertUser(t testing.TB, db *sql.DB, name, email string, phone *string) dbsqlite.User {
	t.Helper()

	var p sql.NullString
	if phone != nil {
		p = sql.NullString{String: *phone, Valid: true}
	}
	u, err := dbsqlite.New(db).CreateUser(context.Background(), dbsqlite.CreateUserParams{
		Name:  name,
		Email: email,
		Phone: p,
	})
	if err != nil {
		t.Fatalf("insertUser: %v", err)
	}
	return u
}

func insertOrder(t testing.TB, db *sql.DB, userID int64, createdAt time.Time) int64 {
	t.Helper()

	// SQLite compares DATETIME text lexicographically, so rows inserted from Go
	// must use the same UTC layout CURRENT_TIMESTAMP writes ("2006-01-02
	// 15:04:05") or ordering breaks. The driver still reads both back as
	// time.Time.
	res, err := db.Exec(
		"INSERT INTO orders (user_id, amount, created_at) VALUES (?, 1.00, ?)",
		userID, createdAt.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		t.Fatalf("insertOrder: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("insertOrder id: %v", err)
	}
	return id
}

func strPtr(v string) *string { return &v }

// nullStr builds the *sql.NullString a `-- :if` annotated nullable column
// parameter expects: non-nil pointer = filter active.
func nullStr(v string) *sql.NullString {
	return &sql.NullString{String: v, Valid: true}
}

func idSet(users []dbsqlite.User) map[int64]bool {
	ids := make(map[int64]bool, len(users))
	for _, u := range users {
		ids[u.ID] = true
	}
	return ids
}
