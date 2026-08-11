package e2emysql

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"example/dbmysql"
)

func TestGetUserWithLock(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	q := dbmysql.New(db)

	user := insertUser(t, db, "lockuser", "lockuser@example.com", nil)

	t.Run("WithoutLock", func(t *testing.T) {
		got, err := q.GetUserWithLock(ctx, dbmysql.GetUserWithLockParams{
			ID:   user.ID,
			Lock: false,
		})
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != user.ID {
			t.Errorf("got %v, want user %d", got, user.ID)
		}
	})

	t.Run("WithLock", func(t *testing.T) {
		// FOR UPDATE holds a row lock until commit; run in a transaction so it
		// is released cleanly.
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()

		got, err := q.WithTx(tx).GetUserWithLock(ctx, dbmysql.GetUserWithLockParams{
			ID:   user.ID,
			Lock: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != user.ID {
			t.Errorf("got %v, want user %d", got, user.ID)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := q.GetUserWithLock(ctx, dbmysql.GetUserWithLockParams{
			ID:   -1,
			Lock: false,
		})
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("got err %v, want sql.ErrNoRows", err)
		}
	})
}
