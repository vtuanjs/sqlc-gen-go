package e2epostgres

import (
	"context"
	"testing"
	"time"

	"example/dbpostgres"
	setup "example/e2e-setup"
)

func TestSearchUsers(t *testing.T) {
	conn := setup.NewDB(t, "../postgres/schema.sql")
	ctx := context.Background()
	q := dbpostgres.NewSearchQueries()

	alice1 := setup.InsertUser(t, conn, "alice", "alice@example.com", setup.StrPtr("+1111111111"))
	alice2 := setup.InsertUser(t, conn, "alice", "alice2@example.com", nil)
	bob := setup.InsertUser(t, conn, "bob", "bob@example.com", nil)
	setup.InsertOrder(t, conn, alice1.ID, time.Now().Add(-24*time.Hour))

	t.Run("NoOptionalFilters", func(t *testing.T) {
		users, err := q.SearchUsers(ctx, conn, dbpostgres.SearchUsersParams{Name: "alice"})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 2 {
			t.Errorf("got %d users, want 2", len(users))
		}
	})

	t.Run("EmailFilter", func(t *testing.T) {
		users, err := q.SearchUsers(ctx, conn, dbpostgres.SearchUsersParams{
			Name:  "alice",
			Email: setup.StrPtr("alice@example.com"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].ID != alice1.ID {
			t.Errorf("got %v, want alice1", users)
		}
	})

	t.Run("PhoneFilter", func(t *testing.T) {
		users, err := q.SearchUsers(ctx, conn, dbpostgres.SearchUsersParams{
			Name:  "alice",
			Phone: setup.StrPtr("+1111111111"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].ID != alice1.ID {
			t.Errorf("got %v, want alice1", users)
		}
	})

	t.Run("HasOrders_False", func(t *testing.T) {
		users, err := q.SearchUsers(ctx, conn, dbpostgres.SearchUsersParams{
			Name:      "alice",
			HasOrders: false,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 2 {
			t.Errorf("got %d users, want 2", len(users))
		}
	})

	t.Run("HasOrders_True", func(t *testing.T) {
		users, err := q.SearchUsers(ctx, conn, dbpostgres.SearchUsersParams{
			Name:      "alice",
			HasOrders: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].ID != alice1.ID {
			t.Errorf("got %v, want only alice1 (has order)", users)
		}
	})

	t.Run("HasOrders_True_WithOrdersSince_Match", func(t *testing.T) {
		since := time.Now().Add(-48 * time.Hour)
		users, err := q.SearchUsers(ctx, conn, dbpostgres.SearchUsersParams{
			Name:        "alice",
			HasOrders:   true,
			OrdersSince: &since,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].ID != alice1.ID {
			t.Errorf("got %v, want alice1", users)
		}
	})

	t.Run("HasOrders_True_WithOrdersSince_NoMatch", func(t *testing.T) {
		since := time.Now().Add(time.Hour) // future — no orders qualify
		users, err := q.SearchUsers(ctx, conn, dbpostgres.SearchUsersParams{
			Name:        "alice",
			HasOrders:   true,
			OrdersSince: &since,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 0 {
			t.Errorf("got %d users, want 0", len(users))
		}
	})

	t.Run("NoMatch", func(t *testing.T) {
		users, err := q.SearchUsers(ctx, conn, dbpostgres.SearchUsersParams{Name: bob.Name})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].ID != bob.ID {
			t.Errorf("got %v, want bob", users)
		}
	})

	_ = alice2
}

func TestSearchUsersOrdered(t *testing.T) {
	conn := setup.NewDB(t, "../postgres/schema.sql")
	ctx := context.Background()
	q := dbpostgres.NewSearchQueries()

	// Insert two alices so ordering is observable
	a1 := setup.InsertUser(t, conn, "alice", "alice.a@example.com", nil)
	a2 := setup.InsertUser(t, conn, "alice", "alice.b@example.com", nil)

	t.Run("NoOrderFlags", func(t *testing.T) {
		users, err := q.SearchUsersOrdered(ctx, conn, dbpostgres.SearchUsersOrderedParams{Name: "alice"})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 2 {
			t.Errorf("got %d users, want 2", len(users))
		}
		// default order is id ASC
		if users[0].ID > users[1].ID {
			t.Errorf("expected id ASC, got %d > %d", users[0].ID, users[1].ID)
		}
	})

	t.Run("EmailFilter", func(t *testing.T) {
		users, err := q.SearchUsersOrdered(ctx, conn, dbpostgres.SearchUsersOrderedParams{
			Name:  "alice",
			Email: setup.StrPtr("alice.a@example.com"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].ID != a1.ID {
			t.Errorf("got %v, want a1", users)
		}
	})

	t.Run("OrderCreatedAtDesc", func(t *testing.T) {
		users, err := q.SearchUsersOrdered(ctx, conn, dbpostgres.SearchUsersOrderedParams{
			Name:               "alice",
			OrderCreatedAtDesc: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 2 {
			t.Errorf("got %d users, want 2", len(users))
		}
	})

	t.Run("OrderNameAsc", func(t *testing.T) {
		users, err := q.SearchUsersOrdered(ctx, conn, dbpostgres.SearchUsersOrderedParams{
			Name:         "alice",
			OrderNameAsc: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 2 {
			t.Errorf("got %d users, want 2", len(users))
		}
	})

	_ = a2
}

func TestSearchUsersByContact(t *testing.T) {
	conn := setup.NewDB(t, "../postgres/schema.sql")
	ctx := context.Background()
	q := dbpostgres.NewSearchQueries()

	alice := setup.InsertUser(t, conn, "alice", "alice@example.com", setup.StrPtr("+1111111111"))
	_ = setup.InsertUser(t, conn, "alice", "other@example.com", setup.StrPtr("+9999999999"))

	t.Run("BothNil", func(t *testing.T) {
		users, err := q.SearchUsersByContact(ctx, conn, dbpostgres.SearchUsersByContactParams{Name: "alice"})
		if err != nil {
			t.Fatal(err)
		}
		// no contact filter → all alices returned
		if len(users) != 2 {
			t.Errorf("got %d users, want 2", len(users))
		}
	})

	t.Run("EmailAndPhone_Match", func(t *testing.T) {
		users, err := q.SearchUsersByContact(ctx, conn, dbpostgres.SearchUsersByContactParams{
			Name:  "alice",
			Email: setup.StrPtr("alice@example.com"),
			Phone: setup.StrPtr("+1111111111"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].ID != alice.ID {
			t.Errorf("got %v, want alice", users)
		}
	})

	t.Run("EmailAndPhone_NoMatch", func(t *testing.T) {
		users, err := q.SearchUsersByContact(ctx, conn, dbpostgres.SearchUsersByContactParams{
			Name:  "alice",
			Email: setup.StrPtr("nobody@example.com"),
			Phone: setup.StrPtr("+0000000000"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 0 {
			t.Errorf("got %d users, want 0", len(users))
		}
	})
}

func TestSearchUsersOrderedByID(t *testing.T) {
	conn := setup.NewDB(t, "../postgres/schema.sql")
	ctx := context.Background()
	q := dbpostgres.NewSearchQueries()

	a1 := setup.InsertUser(t, conn, "alice", "alice.x@example.com", nil)
	a2 := setup.InsertUser(t, conn, "alice", "alice.y@example.com", nil)

	t.Run("NoOrderFlags", func(t *testing.T) {
		// Both flags false → ORDER BY removed entirely, query still valid.
		users, err := q.SearchUsersOrderedByID(ctx, conn, dbpostgres.SearchUsersOrderedByIDParams{
			Name: "alice",
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 2 {
			t.Errorf("got %d users, want 2", len(users))
		}
	})

	t.Run("IDAsc", func(t *testing.T) {
		users, err := q.SearchUsersOrderedByID(ctx, conn, dbpostgres.SearchUsersOrderedByIDParams{
			Name:  "alice",
			IdAsc: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 2 {
			t.Fatalf("got %d users, want 2", len(users))
		}
		if users[0].ID != a1.ID || users[1].ID != a2.ID {
			t.Errorf("expected id ASC order: got [%d, %d]", users[0].ID, users[1].ID)
		}
	})

	t.Run("IDDesc", func(t *testing.T) {
		users, err := q.SearchUsersOrderedByID(ctx, conn, dbpostgres.SearchUsersOrderedByIDParams{
			Name:   "alice",
			IdDesc: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 2 {
			t.Fatalf("got %d users, want 2", len(users))
		}
		if users[0].ID != a2.ID || users[1].ID != a1.ID {
			t.Errorf("expected id DESC order: got [%d, %d]", users[0].ID, users[1].ID)
		}
	})

	t.Run("IDAscAndIDDesc", func(t *testing.T) {
		users, err := q.SearchUsersOrderedByID(ctx, conn, dbpostgres.SearchUsersOrderedByIDParams{
			Name:   "alice",
			IdAsc:  true,
			IdDesc: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 2 {
			t.Errorf("got %d users, want 2", len(users))
		}
	})

	t.Run("WithEmailFilter", func(t *testing.T) {
		users, err := q.SearchUsersOrderedByID(ctx, conn, dbpostgres.SearchUsersOrderedByIDParams{
			Name:   "alice",
			Email:  setup.StrPtr("alice.x@example.com"),
			IdAsc:  true,
			IdDesc: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].ID != a1.ID {
			t.Errorf("got %v, want a1", users)
		}
	})
}

func TestSearchUsersByIDs(t *testing.T) {
	conn := setup.NewDB(t, "../postgres/schema.sql")
	ctx := context.Background()
	q := dbpostgres.NewSearchQueries()

	alice := setup.InsertUser(t, conn, "alice", "alice.ids@example.com", nil)
	bob := setup.InsertUser(t, conn, "alice", "bob.ids@example.com", nil) // same name, different id

	t.Run("NilIDs_ReturnsAll", func(t *testing.T) {
		// nil slice → condition skipped → both users returned
		users, err := q.SearchUsersByIDs(ctx, conn, dbpostgres.SearchUsersByIDsParams{Name: "alice"})
		if err != nil {
			t.Fatal(err)
		}
		ids := make(map[int64]bool, len(users))
		for _, u := range users {
			ids[u.ID] = true
		}
		if !ids[alice.ID] || !ids[bob.ID] {
			t.Errorf("expected both alice and bob when IDs is nil, got %v", users)
		}
	})

	t.Run("SpecificIDs_OnlyAlice", func(t *testing.T) {
		// non-nil slice → condition active → only alice matches
		users, err := q.SearchUsersByIDs(ctx, conn, dbpostgres.SearchUsersByIDsParams{
			Name: "alice",
			Ids:  []int64{alice.ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].ID != alice.ID {
			t.Errorf("got %v, want only alice (%d)", users, alice.ID)
		}
	})

	t.Run("MultipleIDs_BothMatch", func(t *testing.T) {
		users, err := q.SearchUsersByIDs(ctx, conn, dbpostgres.SearchUsersByIDsParams{
			Name: "alice",
			Ids:  []int64{alice.ID, bob.ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 2 {
			t.Errorf("got %d users, want 2", len(users))
		}
	})

	t.Run("EmptySlice_MatchesNothing", func(t *testing.T) {
		// empty non-nil slice → condition active → IN (NULL) → zero rows
		users, err := q.SearchUsersByIDs(ctx, conn, dbpostgres.SearchUsersByIDsParams{
			Name: "alice",
			Ids:  []int64{},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 0 {
			t.Errorf("got %d users, want 0 for empty id list (filter by empty set)", len(users))
		}
	})

	t.Run("NilableSlice_EmptySkipsCondition", func(t *testing.T) {
		// NilableSlice turns empty into nil for callers who want empty to
		// mean "don't filter"
		users, err := q.SearchUsersByIDs(ctx, conn, dbpostgres.SearchUsersByIDsParams{
			Name: "alice",
			Ids:  dbpostgres.NilableSlice([]int64{}),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 2 {
			t.Errorf("got %d users, want 2 for NilableSlice(empty) (clause skipped)", len(users))
		}
	})

	t.Run("IDNotInList_NoMatch", func(t *testing.T) {
		users, err := q.SearchUsersByIDs(ctx, conn, dbpostgres.SearchUsersByIDsParams{
			Name: "alice",
			Ids:  []int64{-1},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 0 {
			t.Errorf("got %d users, want 0", len(users))
		}
	})
}

func TestSearchUsersWithSameNameAndEmail(t *testing.T) {
	conn := setup.NewDB(t, "../postgres/schema.sql")
	ctx := context.Background()
	q := dbpostgres.NewSearchQueries()

	// "dual" user: name and email are the same string — matches both conditions.
	dual := setup.InsertUser(t, conn, "dual", "dual", nil)
	// "normal" user: name matches the search term but email differs — never matches.
	normal := setup.InsertUser(t, conn, "dual", "dual@example.com", nil)

	t.Run("NameNil_ReturnsAll", func(t *testing.T) {
		// No filter applied → all users in the table (scoped to this test's DB).
		users, err := q.SearchUsersWithSameNameAndEmail(ctx, conn, dbpostgres.SearchUsersWithSameNameAndEmailParams{})
		if err != nil {
			t.Fatal(err)
		}
		ids := make(map[int64]bool, len(users))
		for _, u := range users {
			ids[u.ID] = true
		}
		if !ids[dual.ID] || !ids[normal.ID] {
			t.Errorf("expected both dual and normal to be returned when Name is nil")
		}
	})

	t.Run("NameProvided_OnlyDualUser", func(t *testing.T) {
		// Both name = $1 AND email = $1 must hold — only dual qualifies.
		users, err := q.SearchUsersWithSameNameAndEmail(ctx, conn, dbpostgres.SearchUsersWithSameNameAndEmailParams{
			Name: setup.StrPtr("dual"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].ID != dual.ID {
			t.Errorf("got %v, want only dual user (%d)", users, dual.ID)
		}
	})

	t.Run("NameProvided_NoMatch", func(t *testing.T) {
		// No user has both name="nobody" and email="nobody".
		users, err := q.SearchUsersWithSameNameAndEmail(ctx, conn, dbpostgres.SearchUsersWithSameNameAndEmailParams{
			Name: setup.StrPtr("nobody"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 0 {
			t.Errorf("got %d users, want 0", len(users))
		}
	})

	t.Run("NormalUser_NotReturned", func(t *testing.T) {
		// Confirm normal (name="dual", email="dual@example.com") is excluded
		// when the filter is active — email doesn't match the search value.
		users, err := q.SearchUsersWithSameNameAndEmail(ctx, conn, dbpostgres.SearchUsersWithSameNameAndEmailParams{
			Name: setup.StrPtr("dual"),
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, u := range users {
			if u.ID == normal.ID {
				t.Errorf("normal user (email≠name) should not be returned")
			}
		}
	})
}
