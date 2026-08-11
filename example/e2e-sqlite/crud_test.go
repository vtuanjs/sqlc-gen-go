package e2esqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"example/dbsqlite"
)

func TestUserCRUD(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	q := dbsqlite.New(db)

	// SQLite supports RETURNING, so the insert comes back as a full row.
	user, err := q.CreateUser(ctx, dbsqlite.CreateUserParams{
		Name:  "crud",
		Email: "crud@example.com",
		Phone: sql.NullString{String: "+1234567890", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if user.ID == 0 {
		t.Fatal("CreateUser: got id 0")
	}
	if user.CreatedAt.IsZero() {
		t.Error("CreatedAt should be populated by the column default")
	}

	t.Run("GetUser", func(t *testing.T) {
		u, err := q.GetUser(ctx, user.ID)
		if err != nil {
			t.Fatal(err)
		}
		if u.Name != "crud" || u.Email != "crud@example.com" {
			t.Errorf("got %+v, want name=crud email=crud@example.com", u)
		}
		if !u.Phone.Valid || u.Phone.String != "+1234567890" {
			t.Errorf("got phone %+v, want +1234567890", u.Phone)
		}
	})

	t.Run("UpdateUser", func(t *testing.T) {
		updated, err := q.UpdateUser(ctx, dbsqlite.UpdateUserParams{
			Name:  "crud2",
			Email: "crud2@example.com",
			ID:    user.ID,
		})
		if err != nil {
			t.Fatal(err)
		}
		if updated.Name != "crud2" || updated.Email != "crud2@example.com" {
			t.Errorf("got %+v, want the updated name/email", updated)
		}
		// RETURNING keeps the untouched columns intact
		if updated.Phone != user.Phone {
			t.Errorf("got phone %+v, want %+v", updated.Phone, user.Phone)
		}
	})

	t.Run("ListUsers_SqlcSlice", func(t *testing.T) {
		other := insertUser(t, db, "crud3", "crud3@example.com", nil)

		users, err := q.ListUsers(ctx, []string{"crud2", "crud3"})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 2 {
			t.Fatalf("got %d users, want 2", len(users))
		}
		// ORDER BY name
		if users[0].ID != user.ID || users[1].ID != other.ID {
			t.Errorf("got ids [%d %d], want [%d %d]", users[0].ID, users[1].ID, user.ID, other.ID)
		}
	})

	t.Run("CountUsers", func(t *testing.T) {
		n, err := q.CountUsers(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if n != 2 {
			t.Errorf("got %d users, want 2", n)
		}
	})

	t.Run("DeleteUser", func(t *testing.T) {
		if err := q.DeleteUser(ctx, user.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := q.GetUser(ctx, user.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("got err %v, want sql.ErrNoRows after delete", err)
		}
	})
}

func TestOrderQueries(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	q := dbsqlite.New(db)

	user := insertUser(t, db, "orderuser", "orderuser@example.com", nil)

	// :execlastid returns the rowid SQLite assigned.
	orderID, err := q.CreateOrder(ctx, dbsqlite.CreateOrderParams{
		UserID: user.ID,
		Amount: 19.99,
		Status: "pending",
	})
	if err != nil {
		t.Fatal(err)
	}
	// a second, older order
	insertOrder(t, db, user.ID, time.Now().Add(-time.Hour))

	t.Run("GetOrder", func(t *testing.T) {
		o, err := q.GetOrder(ctx, orderID)
		if err != nil {
			t.Fatal(err)
		}
		if o.UserID != user.ID || o.Amount != 19.99 || o.Status != "pending" {
			t.Errorf("got %+v, want the order just created", o)
		}
	})

	t.Run("UpdateOrderStatus", func(t *testing.T) {
		// :execrows reports how many rows the UPDATE touched.
		n, err := q.UpdateOrderStatus(ctx, dbsqlite.UpdateOrderStatusParams{
			Status: "shipped",
			ID:     orderID,
		})
		if err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("got %d affected rows, want 1", n)
		}

		n, err = q.UpdateOrderStatus(ctx, dbsqlite.UpdateOrderStatusParams{
			Status: "shipped",
			ID:     -1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("got %d affected rows for a missing order, want 0", n)
		}
	})

	t.Run("ListOrdersByUser", func(t *testing.T) {
		orders, err := q.ListOrdersByUser(ctx, user.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(orders) != 2 {
			t.Fatalf("got %d orders, want 2", len(orders))
		}
		// ORDER BY created_at DESC — the older order comes last.
		if orders[0].CreatedAt.Before(orders[1].CreatedAt) {
			t.Errorf("expected created_at DESC, got %v then %v", orders[0].CreatedAt, orders[1].CreatedAt)
		}
	})

	t.Run("GetUserOrderSummary", func(t *testing.T) {
		summary, err := q.GetUserOrderSummary(ctx, user.ID)
		if err != nil {
			t.Fatal(err)
		}
		if summary.Name != "orderuser" {
			t.Errorf("got name %q, want orderuser", summary.Name)
		}
		if summary.OrderCount != 2 {
			t.Errorf("got order_count %d, want 2", summary.OrderCount)
		}
		// COALESCE(SUM(...)) has no single column type, so sqlc scans it as
		// interface{} and the driver decides the concrete type.
		got, ok := summary.TotalSpent.(float64)
		if !ok {
			t.Fatalf("got total_spent %T, want float64", summary.TotalSpent)
		}
		if got != 20.99 {
			t.Errorf("got total_spent %v, want 20.99", got)
		}
	})

	t.Run("NoOrders", func(t *testing.T) {
		empty := insertUser(t, db, "noorders", "noorders@example.com", nil)

		orders, err := q.ListOrdersByUser(ctx, empty.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(orders) != 0 {
			t.Errorf("got %d orders, want 0", len(orders))
		}

		summary, err := q.GetUserOrderSummary(ctx, empty.ID)
		if err != nil {
			t.Fatal(err)
		}
		if summary.OrderCount != 0 {
			t.Errorf("got order_count %d, want 0", summary.OrderCount)
		}
	})
}

func TestProductQueries(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	q := dbsqlite.New(db)

	product, err := q.CreateProduct(ctx, dbsqlite.CreateProductParams{
		Name:  sql.NullString{String: "widget", Valid: true},
		Price: 9.99,
		Stock: sql.NullInt64{Int64: 5, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// nullable columns left NULL
	nullProduct, err := q.CreateProduct(ctx, dbsqlite.CreateProductParams{Price: 1.00})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("GetProduct", func(t *testing.T) {
		p, err := q.GetProduct(ctx, product.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !p.Name.Valid || p.Name.String != "widget" {
			t.Errorf("got name %+v, want widget", p.Name)
		}
		if p.Price != 9.99 {
			t.Errorf("got price %v, want 9.99", p.Price)
		}
	})

	t.Run("NullableColumns", func(t *testing.T) {
		p, err := q.GetProduct(ctx, nullProduct.ID)
		if err != nil {
			t.Fatal(err)
		}
		if p.Name.Valid {
			t.Errorf("got name %+v, want NULL", p.Name)
		}
		if p.Stock.Valid {
			t.Errorf("got stock %+v, want NULL", p.Stock)
		}
	})

	t.Run("GetProductPrice", func(t *testing.T) {
		price, err := q.GetProductPrice(ctx, product.ID)
		if err != nil {
			t.Fatal(err)
		}
		if price != 9.99 {
			t.Errorf("got %v, want 9.99", price)
		}
	})

	t.Run("UpdateProductStock", func(t *testing.T) {
		err := q.UpdateProductStock(ctx, dbsqlite.UpdateProductStockParams{
			Stock: sql.NullInt64{Int64: 42, Valid: true},
			ID:    product.ID,
		})
		if err != nil {
			t.Fatal(err)
		}
		p, err := q.GetProduct(ctx, product.ID)
		if err != nil {
			t.Fatal(err)
		}
		if p.Stock.Int64 != 42 {
			t.Errorf("got stock %+v, want 42", p.Stock)
		}
	})

	t.Run("GetProductsInStock", func(t *testing.T) {
		stock, err := q.GetProductsInStock(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !stock.Valid || stock.Int64 <= 0 {
			t.Errorf("got %+v, want a positive stock value", stock)
		}
	})

	t.Run("ListProducts", func(t *testing.T) {
		products, err := q.ListProducts(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(products) != 2 {
			t.Errorf("got %d products, want 2", len(products))
		}
	})

	t.Run("DeleteProduct", func(t *testing.T) {
		if err := q.DeleteProduct(ctx, nullProduct.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := q.GetProduct(ctx, nullProduct.ID); !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("got err %v, want sql.ErrNoRows after delete", err)
		}
	})
}
