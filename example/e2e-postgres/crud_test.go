package e2epostgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"example/dbpostgres"
	setup "example/e2e-setup"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

func TestUserCRUD(t *testing.T) {
	conn := setup.NewDB(t, "../postgres/schema.sql")
	ctx := context.Background()
	q := dbpostgres.NewUsersQueries()

	// PostgreSQL supports RETURNING, so the insert comes back as a full row.
	user, err := q.CreateUser(ctx, conn, dbpostgres.CreateUserParams{
		Name:  "crud",
		Email: "crud@example.com",
		Phone: setup.StrPtr("+1234567890"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if user == nil || user.ID == 0 {
		t.Fatal("CreateUser: got no row")
	}
	t.Cleanup(func() {
		conn.Exec(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
	})
	if user.CreatedAt.IsZero() {
		t.Error("CreatedAt should be populated by the column default")
	}

	t.Run("GetUser", func(t *testing.T) {
		u, err := q.GetUser(ctx, conn, dbpostgres.GetUserParams{ID: user.ID})
		if err != nil {
			t.Fatal(err)
		}
		if u == nil || u.Name != "crud" || u.Email != "crud@example.com" {
			t.Fatalf("got %+v, want name=crud email=crud@example.com", u)
		}
		if u.Phone == nil || *u.Phone != "+1234567890" {
			t.Errorf("got phone %v, want +1234567890", u.Phone)
		}
	})

	t.Run("UpdateUser", func(t *testing.T) {
		updated, err := q.UpdateUser(ctx, conn, dbpostgres.UpdateUserParams{
			Name:  "crud2",
			Email: "crud2@example.com",
			ID:    user.ID,
		})
		if err != nil {
			t.Fatal(err)
		}
		if updated == nil || updated.Name != "crud2" || updated.Email != "crud2@example.com" {
			t.Fatalf("got %+v, want the updated name/email", updated)
		}
		// RETURNING keeps the untouched columns intact
		if updated.Phone == nil || *updated.Phone != "+1234567890" {
			t.Errorf("got phone %v, want it preserved", updated.Phone)
		}
	})

	t.Run("ListUsers_AnyArray", func(t *testing.T) {
		other := setup.InsertUser(t, conn, "crud3", "crud3@example.com", nil)

		users, err := q.ListUsers(ctx, conn, dbpostgres.ListUsersParams{
			Names: []string{"crud2", "crud3"},
		})
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
		n, err := q.CountUsers(ctx, conn)
		if err != nil {
			t.Fatal(err)
		}
		if n < 1 {
			t.Errorf("got %d users, want at least 1", n)
		}
	})

	t.Run("DeleteUser", func(t *testing.T) {
		if err := q.DeleteUser(ctx, conn, dbpostgres.DeleteUserParams{ID: user.ID}); err != nil {
			t.Fatal(err)
		}
		// emit_err_nil_if_no_rows: a missing row is (nil, nil), not ErrNoRows.
		got, err := q.GetUser(ctx, conn, dbpostgres.GetUserParams{ID: user.ID})
		if err != nil {
			t.Fatalf("got err %v, want nil (emit_err_nil_if_no_rows)", err)
		}
		if got != nil {
			t.Errorf("got %+v, want nil after delete", got)
		}
	})
}

func TestOrderQueries(t *testing.T) {
	conn := setup.NewDB(t, "../postgres/schema.sql")
	ctx := context.Background()
	q := dbpostgres.NewOrdersQueries()

	user := setup.InsertUser(t, conn, "orderuser", "orderuser@example.com", nil)

	order, err := q.CreateOrder(ctx, conn, dbpostgres.CreateOrderParams{
		UserID: user.ID,
		Amount: decimal.RequireFromString("19.99"),
		Status: "pending",
	})
	if err != nil {
		t.Fatal(err)
	}
	if order == nil {
		t.Fatal("CreateOrder: got no row")
	}
	t.Cleanup(func() {
		conn.Exec(context.Background(), "DELETE FROM orders WHERE id = $1", order.ID)
	})

	// a second, older order
	setup.InsertOrder(t, conn, user.ID, time.Now().Add(-time.Hour))

	t.Run("GetOrder", func(t *testing.T) {
		o, err := q.GetOrder(ctx, conn, dbpostgres.GetOrderParams{ID: order.ID})
		if err != nil {
			t.Fatal(err)
		}
		if o == nil || o.UserID != user.ID || o.Status != "pending" {
			t.Fatalf("got %+v, want the order just created", o)
		}
		if !o.Amount.Equal(decimal.RequireFromString("19.99")) {
			t.Errorf("got amount %s, want 19.99", o.Amount)
		}
	})

	t.Run("UpdateOrderStatus", func(t *testing.T) {
		// :execrows reports how many rows the UPDATE touched.
		n, err := q.UpdateOrderStatus(ctx, conn, dbpostgres.UpdateOrderStatusParams{
			Status: "shipped",
			ID:     order.ID,
		})
		if err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("got %d affected rows, want 1", n)
		}

		n, err = q.UpdateOrderStatus(ctx, conn, dbpostgres.UpdateOrderStatusParams{
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
		orders, err := q.ListOrdersByUser(ctx, conn, dbpostgres.ListOrdersByUserParams{UserID: user.ID})
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
		summary, err := q.GetUserOrderSummary(ctx, conn, dbpostgres.GetUserOrderSummaryParams{ID: user.ID})
		if err != nil {
			t.Fatal(err)
		}
		if summary == nil || summary.Name != "orderuser" {
			t.Fatalf("got %+v, want orderuser", summary)
		}
		if summary.OrderCount != 2 {
			t.Errorf("got order_count %d, want 2", summary.OrderCount)
		}
		// COALESCE(SUM(...)) has no single column type, so sqlc scans it as
		// interface{} and the driver decides the concrete type — pgx hands
		// back a pgtype.Numeric rather than the overridden decimal.Decimal.
		num, ok := summary.TotalSpent.(pgtype.Numeric)
		if !ok {
			t.Fatalf("got total_spent %T, want pgtype.Numeric", summary.TotalSpent)
		}
		v, err := num.Value()
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%v", v); got != "20.99" {
			t.Errorf("got total_spent %q, want 20.99", got)
		}
	})

	t.Run("NoOrders", func(t *testing.T) {
		empty := setup.InsertUser(t, conn, "noorders", "noorders@example.com", nil)

		orders, err := q.ListOrdersByUser(ctx, conn, dbpostgres.ListOrdersByUserParams{UserID: empty.ID})
		if err != nil {
			t.Fatal(err)
		}
		if len(orders) != 0 {
			t.Errorf("got %d orders, want 0", len(orders))
		}

		summary, err := q.GetUserOrderSummary(ctx, conn, dbpostgres.GetUserOrderSummaryParams{ID: empty.ID})
		if err != nil {
			t.Fatal(err)
		}
		if summary == nil || summary.OrderCount != 0 {
			t.Errorf("got %+v, want order_count 0", summary)
		}
	})
}

func TestProductQueries(t *testing.T) {
	conn := setup.NewDB(t, "../postgres/schema.sql")
	ctx := context.Background()
	q := dbpostgres.NewProductQueries()

	t.Cleanup(func() { conn.Exec(context.Background(), "DELETE FROM products") })

	product, err := q.CreateProduct(ctx, conn, dbpostgres.CreateProductParams{
		Name:  setup.StrPtr("widget"),
		Price: decimal.RequireFromString("9.99"),
		Stock: setup.I32Ptr(5),
	})
	if err != nil {
		t.Fatal(err)
	}

	// nullable columns left NULL
	nullProduct, err := q.CreateProduct(ctx, conn, dbpostgres.CreateProductParams{
		Price: decimal.RequireFromString("1.00"),
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("GetProduct", func(t *testing.T) {
		p, err := q.GetProduct(ctx, conn, dbpostgres.GetProductParams{ID: product.ID})
		if err != nil {
			t.Fatal(err)
		}
		if p == nil || p.Name == nil || *p.Name != "widget" {
			t.Fatalf("got %+v, want name widget", p)
		}
		if !p.Price.Equal(decimal.RequireFromString("9.99")) {
			t.Errorf("got price %s, want 9.99", p.Price)
		}
	})

	t.Run("NullableColumns", func(t *testing.T) {
		p, err := q.GetProduct(ctx, conn, dbpostgres.GetProductParams{ID: nullProduct.ID})
		if err != nil {
			t.Fatal(err)
		}
		if p == nil {
			t.Fatal("got no row")
		}
		if p.Name != nil {
			t.Errorf("got name %v, want NULL", *p.Name)
		}
		if p.Stock != nil {
			t.Errorf("got stock %v, want NULL", *p.Stock)
		}
	})

	t.Run("GetProductPrice", func(t *testing.T) {
		price, err := q.GetProductPrice(ctx, conn, dbpostgres.GetProductPriceParams{ID: product.ID})
		if err != nil {
			t.Fatal(err)
		}
		if !price.Equal(decimal.RequireFromString("9.99")) {
			t.Errorf("got %s, want 9.99", price)
		}
	})

	t.Run("UpdateProductStock", func(t *testing.T) {
		err := q.UpdateProductStock(ctx, conn, dbpostgres.UpdateProductStockParams{
			Stock: setup.I32Ptr(42),
			ID:    product.ID,
		})
		if err != nil {
			t.Fatal(err)
		}
		p, err := q.GetProduct(ctx, conn, dbpostgres.GetProductParams{ID: product.ID})
		if err != nil {
			t.Fatal(err)
		}
		if p.Stock == nil || *p.Stock != 42 {
			t.Errorf("got stock %v, want 42", p.Stock)
		}
	})

	t.Run("GetProductsInStock", func(t *testing.T) {
		stock, err := q.GetProductsInStock(ctx, conn)
		if err != nil {
			t.Fatal(err)
		}
		if stock == nil || *stock <= 0 {
			t.Errorf("got %v, want a positive stock value", stock)
		}
	})

	t.Run("ListProducts", func(t *testing.T) {
		products, err := q.ListProducts(ctx, conn)
		if err != nil {
			t.Fatal(err)
		}
		if len(products) != 2 {
			t.Errorf("got %d products, want 2", len(products))
		}
	})

	t.Run("DeleteProduct", func(t *testing.T) {
		err := q.DeleteProduct(ctx, conn, dbpostgres.DeleteProductParams{ID: nullProduct.ID})
		if err != nil {
			t.Fatal(err)
		}
		got, err := q.GetProduct(ctx, conn, dbpostgres.GetProductParams{ID: nullProduct.ID})
		if err != nil {
			t.Fatalf("got err %v, want nil (emit_err_nil_if_no_rows)", err)
		}
		if got != nil {
			t.Errorf("got %+v, want nil after delete", got)
		}
	})
}
