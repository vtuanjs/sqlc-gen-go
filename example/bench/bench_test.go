package bench

import (
	"example/dbpostgres"
	"fmt"
	"strings"
	"testing"
	"time"
)

// largeQuery simulates a query with 20 optional filter conditions.
// $1 is the required name; $2–$21 are optional string filters.
const largeQuery = `-- name: LargeSearch :many
SELECT id, name, email, created_at, phone FROM users
WHERE name = $1
  AND f1 = $2 -- :if $2
  AND f2 = $3 -- :if $3
  AND f3 = $4 -- :if $4
  AND f4 = $5 -- :if $5
  AND f5 = $6 -- :if $6
  AND f6 = $7 -- :if $7
  AND f7 = $8 -- :if $8
  AND f8 = $9 -- :if $9
  AND f9 = $10 -- :if $10
  AND f10 = $11 -- :if $11
  AND f11 = $12 -- :if $12
  AND f12 = $13 -- :if $13
  AND f13 = $14 -- :if $14
  AND f14 = $15 -- :if $15
  AND f15 = $16 -- :if $16
  AND f16 = $17 -- :if $17
  AND f17 = $18 -- :if $18
  AND f18 = $19 -- :if $19
  AND f19 = $20 -- :if $20
  AND f20 = $21 -- :if $21
ORDER BY id ASC
`

func manualLargeSearch(name string, filters [20]*string) (string, []any) {
	var sb strings.Builder
	args := make([]any, 0, 21)
	idx := 1

	sb.WriteString("SELECT id, name, email, created_at, phone FROM users\nWHERE name = $1")
	args = append(args, name)
	idx++

	fields := [20]string{
		"f1", "f2", "f3", "f4", "f5",
		"f6", "f7", "f8", "f9", "f10",
		"f11", "f12", "f13", "f14", "f15",
		"f16", "f17", "f18", "f19", "f20",
	}
	for i, f := range filters {
		if f != nil {
			fmt.Fprintf(&sb, "\n  AND %s = $%d", fields[i], idx)
			args = append(args, f)
			idx++
		}
	}
	sb.WriteString("\nORDER BY id ASC")
	return sb.String(), args
}

func makeLargeArgs(setAll bool) []any {
	args := make([]any, 21)
	args[0] = "alice"
	for i := 1; i <= 20; i++ {
		if setAll {
			v := fmt.Sprintf("val%d", i)
			args[i] = &v
		} else {
			args[i] = (*string)(nil)
		}
	}
	return args
}

func makeLargeFilters(setAll bool) [20]*string {
	var filters [20]*string
	if setAll {
		for i := range filters {
			v := fmt.Sprintf("val%d", i+1)
			filters[i] = &v
		}
	}
	return filters
}

func strPtr(v string) *string { return &v }

func manualSearchUsers(name string, email, phone *string, ordersSince *time.Time, hasOrders bool) (string, []any) {
	var sb strings.Builder
	args := make([]any, 0, 5)
	idx := 1

	sb.WriteString("SELECT id, name, email, created_at, phone FROM users\nWHERE name = $1")
	args = append(args, name)
	idx++

	if email != nil {
		fmt.Fprintf(&sb, "\n  AND email = $%d", idx)
		args = append(args, email)
		idx++
	}
	if phone != nil {
		fmt.Fprintf(&sb, "\n  AND phone = $%d", idx)
		args = append(args, phone)
		idx++
	}
	if hasOrders {
		if ordersSince != nil {
			fmt.Fprintf(&sb, "\n  AND EXISTS (\n    SELECT 1 FROM orders\n    WHERE orders.user_id = users.id\n      AND orders.created_at >= $%d\n  )", idx)
			args = append(args, ordersSince)
		} else {
			sb.WriteString("\n  AND EXISTS (\n    SELECT 1 FROM orders\n    WHERE orders.user_id = users.id\n  )")
		}
	}
	sb.WriteString("\nORDER BY id ASC")
	return sb.String(), args
}

var (
	benchEmail = strPtr("alice@example.com")
	benchPhone = strPtr("+1234567890")
	benchTime  = func() *time.Time { t := time.Now(); return &t }()
)

func BenchmarkDynamicSQL_NoOptional(b *testing.B) {
	for i := 0; i < b.N; i++ {
		dbpostgres.DynamicSQL(dbpostgres.SearchUsers, []any{"alice", (*string)(nil), (*string)(nil), (*time.Time)(nil), false})
	}
}

func BenchmarkManual_NoOptional(b *testing.B) {
	for i := 0; i < b.N; i++ {
		manualSearchUsers("alice", nil, nil, nil, false)
	}
}

func BenchmarkDynamicSQL_AllOptional(b *testing.B) {
	for i := 0; i < b.N; i++ {
		dbpostgres.DynamicSQL(dbpostgres.SearchUsers, []any{"alice", benchEmail, benchPhone, benchTime, true})
	}
}

func BenchmarkManual_AllOptional(b *testing.B) {
	for i := 0; i < b.N; i++ {
		manualSearchUsers("alice", benchEmail, benchPhone, benchTime, true)
	}
}

// Large query benchmarks (20 optional conditions).

var (
	largeArgsNone = makeLargeArgs(false)
	largeArgsAll  = makeLargeArgs(true)
	largeFiltNone = makeLargeFilters(false)
	largeFiltAll  = makeLargeFilters(true)
)

// preCompiledSearchUsers simulates the generated package-level var:
//
//	var _searchUsersDynQ = dynCompile(SearchUsersSQL)
var preCompiledSearchUsers = dbpostgres.CompileDynSQL(dbpostgres.SearchUsers)
var preCompiledLargeQuery = dbpostgres.CompileDynSQL(largeQuery)

func BenchmarkDynamicSQL_Large_NoOptional(b *testing.B) {
	for i := 0; i < b.N; i++ {
		dbpostgres.DynamicSQL(largeQuery, largeArgsNone)
	}
}

func BenchmarkPreCompiled_Large_NoOptional(b *testing.B) {
	for i := 0; i < b.N; i++ {
		preCompiledLargeQuery.Build(largeArgsNone)
	}
}

func BenchmarkManual_Large_NoOptional(b *testing.B) {
	for i := 0; i < b.N; i++ {
		manualLargeSearch("alice", largeFiltNone)
	}
}

func BenchmarkDynamicSQL_Large_AllOptional(b *testing.B) {
	for i := 0; i < b.N; i++ {
		dbpostgres.DynamicSQL(largeQuery, largeArgsAll)
	}
}

func BenchmarkPreCompiled_Large_AllOptional(b *testing.B) {
	for i := 0; i < b.N; i++ {
		preCompiledLargeQuery.Build(largeArgsAll)
	}
}

func BenchmarkManual_Large_AllOptional(b *testing.B) {
	for i := 0; i < b.N; i++ {
		manualLargeSearch("alice", largeFiltAll)
	}
}

func BenchmarkDynamicSQL_NoOptional2(b *testing.B) {
	for i := 0; i < b.N; i++ {
		dbpostgres.DynamicSQL(dbpostgres.SearchUsers, []any{"alice", (*string)(nil), (*string)(nil), (*time.Time)(nil), false})
	}
}

func BenchmarkPreCompiled_NoOptional(b *testing.B) {
	for i := 0; i < b.N; i++ {
		preCompiledSearchUsers.Build([]any{"alice", (*string)(nil), (*string)(nil), (*time.Time)(nil), false})
	}
}

func BenchmarkPreCompiled_AllOptional(b *testing.B) {
	for i := 0; i < b.N; i++ {
		preCompiledSearchUsers.Build([]any{"alice", benchEmail, benchPhone, benchTime, true})
	}
}
