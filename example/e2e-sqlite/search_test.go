package e2esqlite

import (
	"context"
	"testing"
	"time"

	"example/dbsqlite"
)

func TestSearchUsers(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	q := dbsqlite.New(db)

	alice1 := insertUser(t, db, "alice", "alice@example.com", strPtr("+1111111111"))
	insertUser(t, db, "alice", "alice2@example.com", nil)
	bob := insertUser(t, db, "bob", "bob@example.com", nil)
	insertOrder(t, db, alice1.ID, time.Now().Add(-24*time.Hour))

	t.Run("NoOptionalFilters", func(t *testing.T) {
		users, err := q.SearchUsers(ctx, dbsqlite.SearchUsersParams{Name: "alice"})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 2 {
			t.Errorf("got %d users, want 2", len(users))
		}
	})

	t.Run("EmailFilter", func(t *testing.T) {
		users, err := q.SearchUsers(ctx, dbsqlite.SearchUsersParams{
			Name:  "alice",
			Email: strPtr("alice@example.com"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].ID != alice1.ID {
			t.Errorf("got %v, want alice1", users)
		}
	})

	t.Run("Nilable/EmptyEmailSkipsFilter", func(t *testing.T) {
		// Nilable turns the zero value into nil, so an unfilled form field
		// leaves the clause out instead of matching on "".
		users, err := q.SearchUsers(ctx, dbsqlite.SearchUsersParams{
			Name:  "alice",
			Email: dbsqlite.Nilable(""),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 2 {
			t.Errorf("got %d users, want 2 (clause skipped)", len(users))
		}
	})

	t.Run("Nilable/NonEmptyEmailFilters", func(t *testing.T) {
		users, err := q.SearchUsers(ctx, dbsqlite.SearchUsersParams{
			Name:  "alice",
			Email: dbsqlite.Nilable("alice@example.com"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].ID != alice1.ID {
			t.Errorf("got %v, want alice1", users)
		}
	})

	t.Run("PhoneFilter", func(t *testing.T) {
		users, err := q.SearchUsers(ctx, dbsqlite.SearchUsersParams{
			Name:  "alice",
			Phone: nullStr("+1111111111"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].ID != alice1.ID {
			t.Errorf("got %v, want alice1", users)
		}
	})

	t.Run("HasOrders_False", func(t *testing.T) {
		users, err := q.SearchUsers(ctx, dbsqlite.SearchUsersParams{
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
		users, err := q.SearchUsers(ctx, dbsqlite.SearchUsersParams{
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
		users, err := q.SearchUsers(ctx, dbsqlite.SearchUsersParams{
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
		users, err := q.SearchUsers(ctx, dbsqlite.SearchUsersParams{
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

	t.Run("OrdersSinceWithoutHasOrders_Ignored", func(t *testing.T) {
		// orders_since lives inside the EXISTS block; with has_orders false the
		// whole block is dropped and the date is never bound.
		since := time.Now().Add(time.Hour)
		users, err := q.SearchUsers(ctx, dbsqlite.SearchUsersParams{
			Name:        "alice",
			OrdersSince: &since,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 2 {
			t.Errorf("got %d users, want 2", len(users))
		}
	})

	t.Run("NoMatch", func(t *testing.T) {
		users, err := q.SearchUsers(ctx, dbsqlite.SearchUsersParams{Name: bob.Name})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].ID != bob.ID {
			t.Errorf("got %v, want bob", users)
		}
	})
}

func TestSearchUsersOrdered(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	q := dbsqlite.New(db)

	a1 := insertUser(t, db, "alice", "alice.a@example.com", nil)
	insertUser(t, db, "alice", "alice.b@example.com", nil)

	t.Run("NoOrderFlags", func(t *testing.T) {
		users, err := q.SearchUsersOrdered(ctx, dbsqlite.SearchUsersOrderedParams{Name: "alice"})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 2 {
			t.Fatalf("got %d users, want 2", len(users))
		}
		// default order is id ASC
		if users[0].ID > users[1].ID {
			t.Errorf("expected id ASC, got %d > %d", users[0].ID, users[1].ID)
		}
	})

	t.Run("EmailFilter", func(t *testing.T) {
		users, err := q.SearchUsersOrdered(ctx, dbsqlite.SearchUsersOrderedParams{
			Name:  "alice",
			Email: strPtr("alice.a@example.com"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].ID != a1.ID {
			t.Errorf("got %v, want a1", users)
		}
	})

	t.Run("OrderCreatedAtDesc", func(t *testing.T) {
		users, err := q.SearchUsersOrdered(ctx, dbsqlite.SearchUsersOrderedParams{
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
		users, err := q.SearchUsersOrdered(ctx, dbsqlite.SearchUsersOrderedParams{
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
}

func TestSearchUsersByContact(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	q := dbsqlite.New(db)

	alice := insertUser(t, db, "alice", "alice@example.com", strPtr("+1111111111"))
	insertUser(t, db, "alice", "other@example.com", strPtr("+9999999999"))

	t.Run("BothNil", func(t *testing.T) {
		users, err := q.SearchUsersByContact(ctx, dbsqlite.SearchUsersByContactParams{Name: "alice"})
		if err != nil {
			t.Fatal(err)
		}
		// no contact filter → all alices returned
		if len(users) != 2 {
			t.Errorf("got %d users, want 2", len(users))
		}
	})

	t.Run("OnlyEmail_FilterSkipped", func(t *testing.T) {
		// the clause requires BOTH params, so one alone leaves it inactive
		users, err := q.SearchUsersByContact(ctx, dbsqlite.SearchUsersByContactParams{
			Name:  "alice",
			Email: strPtr("alice@example.com"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 2 {
			t.Errorf("got %d users, want 2 (clause needs email AND phone)", len(users))
		}
	})

	t.Run("EmailAndPhone_Match", func(t *testing.T) {
		users, err := q.SearchUsersByContact(ctx, dbsqlite.SearchUsersByContactParams{
			Name:  "alice",
			Email: strPtr("alice@example.com"),
			Phone: nullStr("+1111111111"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].ID != alice.ID {
			t.Errorf("got %v, want alice", users)
		}
	})

	t.Run("EmailAndPhone_NoMatch", func(t *testing.T) {
		users, err := q.SearchUsersByContact(ctx, dbsqlite.SearchUsersByContactParams{
			Name:  "alice",
			Email: strPtr("nobody@example.com"),
			Phone: nullStr("+0000000000"),
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
	db := newDB(t)
	ctx := context.Background()
	q := dbsqlite.New(db)

	a1 := insertUser(t, db, "alice", "alice.x@example.com", nil)
	a2 := insertUser(t, db, "alice", "alice.y@example.com", nil)

	t.Run("NoOrderFlags", func(t *testing.T) {
		// Both flags false → ORDER BY removed entirely, query still valid.
		users, err := q.SearchUsersOrderedByID(ctx, dbsqlite.SearchUsersOrderedByIDParams{
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
		users, err := q.SearchUsersOrderedByID(ctx, dbsqlite.SearchUsersOrderedByIDParams{
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
		users, err := q.SearchUsersOrderedByID(ctx, dbsqlite.SearchUsersOrderedByIDParams{
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
		users, err := q.SearchUsersOrderedByID(ctx, dbsqlite.SearchUsersOrderedByIDParams{
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
		users, err := q.SearchUsersOrderedByID(ctx, dbsqlite.SearchUsersOrderedByIDParams{
			Name:   "alice",
			Email:  strPtr("alice.x@example.com"),
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
	db := newDB(t)
	ctx := context.Background()
	q := dbsqlite.New(db)

	alice := insertUser(t, db, "alice", "alice.ids@example.com", nil)
	bob := insertUser(t, db, "alice", "bob.ids@example.com", nil) // same name, different id

	t.Run("NilIDs_ReturnsAll", func(t *testing.T) {
		// nil slice → condition skipped → both users returned
		users, err := q.SearchUsersByIDs(ctx, dbsqlite.SearchUsersByIDsParams{Name: "alice"})
		if err != nil {
			t.Fatal(err)
		}
		ids := idSet(users)
		if !ids[alice.ID] || !ids[bob.ID] {
			t.Errorf("expected both alice and bob when IDs is nil, got %v", users)
		}
	})

	t.Run("SpecificIDs_OnlyAlice", func(t *testing.T) {
		users, err := q.SearchUsersByIDs(ctx, dbsqlite.SearchUsersByIDsParams{
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
		users, err := q.SearchUsersByIDs(ctx, dbsqlite.SearchUsersByIDsParams{
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
		users, err := q.SearchUsersByIDs(ctx, dbsqlite.SearchUsersByIDsParams{
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
		users, err := q.SearchUsersByIDs(ctx, dbsqlite.SearchUsersByIDsParams{
			Name: "alice",
			Ids:  dbsqlite.NilableSlice([]int64{}),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 2 {
			t.Errorf("got %d users, want 2 for NilableSlice(empty) (clause skipped)", len(users))
		}
	})

	t.Run("IDNotInList_NoMatch", func(t *testing.T) {
		users, err := q.SearchUsersByIDs(ctx, dbsqlite.SearchUsersByIDsParams{
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
	db := newDB(t)
	ctx := context.Background()
	q := dbsqlite.New(db)

	// "dual" user: name and email are the same string — matches both conditions.
	dual := insertUser(t, db, "dual", "dual", nil)
	// "normal" user: name matches the search term but email differs.
	normal := insertUser(t, db, "dual", "dual@example.com", nil)

	t.Run("NameNil_ReturnsAll", func(t *testing.T) {
		users, err := q.SearchUsersWithSameNameAndEmail(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		ids := idSet(users)
		if !ids[dual.ID] || !ids[normal.ID] {
			t.Errorf("expected both dual and normal to be returned when Name is nil")
		}
	})

	t.Run("NameProvided_OnlyDualUser", func(t *testing.T) {
		// Both name = ?1 AND email = ?1 must hold, bound from the same param.
		users, err := q.SearchUsersWithSameNameAndEmail(ctx, strPtr("dual"))
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].ID != dual.ID {
			t.Errorf("got %v, want only dual user (%d)", users, dual.ID)
		}
	})

	t.Run("NameProvided_NoMatch", func(t *testing.T) {
		users, err := q.SearchUsersWithSameNameAndEmail(ctx, strPtr("nobody"))
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 0 {
			t.Errorf("got %d users, want 0", len(users))
		}
	})
}

func TestSearchUsersWithBlock(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	q := dbsqlite.New(db)

	dual := insertUser(t, db, "dual", "dual", nil)
	other := insertUser(t, db, "other", "other@example.com", nil)

	// Both queries express the same gated block, differing only in where the
	// `-- :if` annotation sits: trailing on the opening paren vs. on its own
	// line above the block.
	t.Run("Trailing/NameNil_BlockDropped", func(t *testing.T) {
		users, err := q.SearchUsersWithBlock(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		ids := idSet(users)
		if !ids[dual.ID] || !ids[other.ID] {
			t.Errorf("expected all users when Name is nil, got %v", users)
		}
	})

	t.Run("Trailing/NameProvided", func(t *testing.T) {
		users, err := q.SearchUsersWithBlock(ctx, strPtr("dual"))
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].ID != dual.ID {
			t.Errorf("got %v, want dual", users)
		}
	})

	t.Run("TopStyle/NameNil_BlockDropped", func(t *testing.T) {
		users, err := q.SearchUsersWithTopStyle(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		ids := idSet(users)
		if !ids[dual.ID] || !ids[other.ID] {
			t.Errorf("expected all users when Name is nil, got %v", users)
		}
	})

	t.Run("TopStyle/NameProvided", func(t *testing.T) {
		users, err := q.SearchUsersWithTopStyle(ctx, strPtr("dual"))
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].ID != dual.ID {
			t.Errorf("got %v, want dual", users)
		}
	})
}

// TestSearchUsersWithPhone covers a flag-only parameter: with_phone gates a
// clause that binds no value. SQLite has no FOR UPDATE, so this stands in for
// the postgres/mysql lock example.
func TestSearchUsersWithPhone(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	q := dbsqlite.New(db)

	withPhone := insertUser(t, db, "alice", "alice.phone@example.com", strPtr("+1111111111"))
	insertUser(t, db, "alice", "alice.nophone@example.com", nil)

	t.Run("FlagOff", func(t *testing.T) {
		users, err := q.SearchUsersWithPhone(ctx, dbsqlite.SearchUsersWithPhoneParams{Name: "alice"})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 2 {
			t.Errorf("got %d users, want 2", len(users))
		}
	})

	t.Run("FlagOn", func(t *testing.T) {
		users, err := q.SearchUsersWithPhone(ctx, dbsqlite.SearchUsersWithPhoneParams{
			Name:      "alice",
			WithPhone: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].ID != withPhone.ID {
			t.Errorf("got %v, want only the user with a phone", users)
		}
	})
}

// TestSearchUsersNestedOptional covers a standalone `-- :if` nested inside an
// already-conditional block: the inner condition gates only its own line, and
// dropping the outer condition removes the whole block with it.
func TestSearchUsersNestedOptional(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	q := dbsqlite.New(db)

	target := insertUser(t, db, "alice", "alice.nested@example.com", strPtr("+1111111111"))
	noPhone := insertUser(t, db, "bob", "bob.nested@example.com", nil)
	other := insertUser(t, db, "carol", "carol.nested@example.com", strPtr("+2222222222"))

	t.Run("EmailNil_WholeBlockDropped", func(t *testing.T) {
		// The inner flag is on, but with the outer condition inactive the whole
		// block — inner line included — disappears.
		users, err := q.SearchUsersNestedOptional(ctx, dbsqlite.SearchUsersNestedOptionalParams{
			AllowNoPhone: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		ids := idSet(users)
		if !ids[target.ID] || !ids[noPhone.ID] || !ids[other.ID] {
			t.Errorf("expected every user when Email is nil, got %v", users)
		}
	})

	t.Run("EmailOnly_InnerLineDropped", func(t *testing.T) {
		users, err := q.SearchUsersNestedOptional(ctx, dbsqlite.SearchUsersNestedOptionalParams{
			Email: strPtr("alice.nested@example.com"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].ID != target.ID {
			t.Errorf("got %v, want only the matching email", users)
		}
	})

	t.Run("EmailAndFlag_InnerLineKept", func(t *testing.T) {
		users, err := q.SearchUsersNestedOptional(ctx, dbsqlite.SearchUsersNestedOptionalParams{
			Email:        strPtr("alice.nested@example.com"),
			AllowNoPhone: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		ids := idSet(users)
		if len(users) != 2 || !ids[target.ID] || !ids[noPhone.ID] {
			t.Errorf("got %v, want the email match plus the phone-less user", users)
		}
	})

	t.Run("FlagOnly_NoEmailMatch", func(t *testing.T) {
		users, err := q.SearchUsersNestedOptional(ctx, dbsqlite.SearchUsersNestedOptionalParams{
			Email:        strPtr("nobody@example.com"),
			AllowNoPhone: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].ID != noPhone.ID {
			t.Errorf("got %v, want only the phone-less user", users)
		}
	})
}

// TestSearchUsersNestedBlock covers a nested standalone `-- :if` that governs a
// whole multi-line sub-block rather than a single line.
func TestSearchUsersNestedBlock(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	q := dbsqlite.New(db)

	alice := insertUser(t, db, "alice", "alice.nb@example.com", nil)
	buyer := insertUser(t, db, "buyer", "buyer.nb@example.com", nil)
	stranger := insertUser(t, db, "stranger", "stranger.nb@example.com", nil)
	insertOrder(t, db, buyer.ID, time.Now().Add(-time.Hour))

	t.Run("NameNil_WholeBlockDropped", func(t *testing.T) {
		users, err := q.SearchUsersNestedBlock(ctx, dbsqlite.SearchUsersNestedBlockParams{
			OrHasOrders: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		ids := idSet(users)
		if !ids[alice.ID] || !ids[buyer.ID] || !ids[stranger.ID] {
			t.Errorf("expected every user when Name is nil, got %v", users)
		}
	})

	t.Run("NameOnly_SubBlockDropped", func(t *testing.T) {
		users, err := q.SearchUsersNestedBlock(ctx, dbsqlite.SearchUsersNestedBlockParams{
			Name: strPtr("alice"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].ID != alice.ID {
			t.Errorf("got %v, want only alice (EXISTS sub-block dropped)", users)
		}
	})

	t.Run("NameAndFlag_SubBlockKept", func(t *testing.T) {
		users, err := q.SearchUsersNestedBlock(ctx, dbsqlite.SearchUsersNestedBlockParams{
			Name:        strPtr("alice"),
			OrHasOrders: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		ids := idSet(users)
		if len(users) != 2 || !ids[alice.ID] || !ids[buyer.ID] {
			t.Errorf("got %v, want alice plus the user with an order", users)
		}
	})
}
