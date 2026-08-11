package db

import (
	"testing"
	"time"

	"example/dbpostgres"
)

func TestNilable(t *testing.T) {
	t.Run("Text/EmptyIsNil", func(t *testing.T) {
		if got := dbpostgres.Nilable(""); got != nil {
			t.Errorf("got %v, want nil", *got)
		}
	})

	t.Run("Text/ValueIsKept", func(t *testing.T) {
		got := dbpostgres.Nilable("alice")
		if got == nil || *got != "alice" {
			t.Errorf("got %v, want alice", got)
		}
	})

	t.Run("Number/ZeroIsNil", func(t *testing.T) {
		if got := dbpostgres.Nilable(int64(0)); got != nil {
			t.Errorf("got %v, want nil", *got)
		}
		if got := dbpostgres.Nilable(0.0); got != nil {
			t.Errorf("got %v, want nil", *got)
		}
	})

	t.Run("Number/ValueIsKept", func(t *testing.T) {
		got := dbpostgres.Nilable(int32(42))
		if got == nil || *got != 42 {
			t.Errorf("got %v, want 42", got)
		}
		neg := dbpostgres.Nilable(-1)
		if neg == nil || *neg != -1 {
			t.Errorf("got %v, want -1", neg)
		}
	})

	t.Run("Bool/FalseIsNil", func(t *testing.T) {
		if got := dbpostgres.Nilable(false); got != nil {
			t.Errorf("got %v, want nil", *got)
		}
		if got := dbpostgres.Nilable(true); got == nil || !*got {
			t.Errorf("got %v, want true", got)
		}
	})

	t.Run("Time/ZeroIsNil", func(t *testing.T) {
		if got := dbpostgres.Nilable(time.Time{}); got != nil {
			t.Errorf("got %v, want nil", *got)
		}
		now := time.Now()
		if got := dbpostgres.Nilable(now); got == nil || !got.Equal(now) {
			t.Errorf("got %v, want %v", got, now)
		}
	})

	t.Run("ReturnsCopy", func(t *testing.T) {
		// The pointer must not alias the caller's variable.
		v := "alice"
		got := dbpostgres.Nilable(v)
		v = "bob"
		if *got != "alice" {
			t.Errorf("got %q, want alice — Nilable must copy", *got)
		}
	})
}

func TestNilableIf(t *testing.T) {
	t.Run("KeepFalseIsNil", func(t *testing.T) {
		if got := dbpostgres.NilableIf("alice", false); got != nil {
			t.Errorf("got %v, want nil", *got)
		}
	})

	t.Run("KeepTrueKeepsZeroValue", func(t *testing.T) {
		// The point of NilableIf: a zero value stays an active filter.
		got := dbpostgres.NilableIf("", true)
		if got == nil || *got != "" {
			t.Errorf("got %v, want a pointer to the empty string", got)
		}
		n := dbpostgres.NilableIf(0, true)
		if n == nil || *n != 0 {
			t.Errorf("got %v, want a pointer to 0", n)
		}
	})
}

func TestPtr(t *testing.T) {
	t.Run("ZeroValueIsNotNil", func(t *testing.T) {
		got := dbpostgres.Ptr("")
		if got == nil || *got != "" {
			t.Errorf("got %v, want a pointer to the empty string", got)
		}
	})

	t.Run("WorksForNonComparable", func(t *testing.T) {
		// Ptr has no comparable constraint, so it accepts types Nilable cannot.
		got := dbpostgres.Ptr([]string{"a"})
		if got == nil || len(*got) != 1 {
			t.Errorf("got %v, want a pointer to [a]", got)
		}
	})
}
