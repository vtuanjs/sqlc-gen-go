package e2esetup

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"example/dbpostgres"

	"github.com/jackc/pgx/v5"
)

const DSN = "postgres://postgres:postgres@localhost:6432/sqlc-test?sslmode=disable"

var (
	migrateOnce sync.Once
	migrateErr  error
)

// NewDB connects to postgres, runs the schema migration once per test binary,
// and registers t.Cleanup to close the connection after the test.
func NewDB(t testing.TB, schemaPath string) *pgx.Conn {
	t.Helper()
	ctx := context.Background()

	conn, err := ConnectWithRetry(ctx, DSN, 10)
	if err != nil {
		t.Fatalf("connect to postgres: %v", err)
	}

	migrateOnce.Do(func() {
		migrateErr = Migrate(ctx, conn, schemaPath)
	})
	if migrateErr != nil {
		conn.Close(ctx)
		t.Fatalf("migrate: %v", migrateErr)
	}

	t.Cleanup(func() { conn.Close(context.Background()) })
	return conn
}

func ConnectWithRetry(ctx context.Context, dsn string, retries int) (*pgx.Conn, error) {
	var err error
	for range retries {
		var c *pgx.Conn
		c, err = pgx.Connect(ctx, dsn)
		if err == nil {
			return c, nil
		}
		time.Sleep(time.Second)
	}
	return nil, err
}

func Migrate(ctx context.Context, c *pgx.Conn, schemaPath string) error {
	drop := `
		DROP TABLE IF EXISTS orders;
		DROP TABLE IF EXISTS products;
		DROP TABLE IF EXISTS users;
	`
	if _, err := c.Exec(ctx, drop); err != nil {
		return fmt.Errorf("drop tables: %w", err)
	}
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("read schema: %w", err)
	}
	if _, err := c.Exec(ctx, string(schema)); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return nil
}

// InsertUser inserts a user and registers cleanup to delete it after the test.
func InsertUser(t testing.TB, conn *pgx.Conn, name, email string, phone *string) *dbpostgres.User {
	t.Helper()
	ctx := context.Background()
	var u dbpostgres.User
	err := conn.QueryRow(ctx,
		`INSERT INTO users (name, email, phone) VALUES ($1, $2, $3)
		 RETURNING id, name, email, created_at, phone`,
		name, email, phone,
	).Scan(&u.ID, &u.Name, &u.Email, &u.CreatedAt, &u.Phone)
	if err != nil {
		t.Fatalf("InsertUser: %v", err)
	}
	t.Cleanup(func() {
		conn.Exec(context.Background(), `DELETE FROM orders WHERE user_id = $1`, u.ID)
		conn.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, u.ID)
	})
	return &u
}

// InsertOrder inserts an order for a user and registers cleanup.
func InsertOrder(t testing.TB, conn *pgx.Conn, userID int64, createdAt time.Time) {
	t.Helper()
	ctx := context.Background()
	var orderID int64
	err := conn.QueryRow(ctx,
		`INSERT INTO orders (user_id, amount, created_at) VALUES ($1, 1.00, $2) RETURNING id`,
		userID, createdAt,
	).Scan(&orderID)
	if err != nil {
		t.Fatalf("InsertOrder: %v", err)
	}
	t.Cleanup(func() {
		conn.Exec(context.Background(), `DELETE FROM orders WHERE id = $1`, orderID)
	})
}

func StrPtr(v string) *string { return &v }
func BoolPtr(v bool) *bool    { return &v }
