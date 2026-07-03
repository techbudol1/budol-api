package store

import "testing"

func TestNormalizePollTimestamp(t *testing.T) {
	t.Run("Manila datetime-local", func(t *testing.T) {
		got, err := normalizePollTimestamp("2026-06-29T14:07")
		if err != nil {
			t.Fatal(err)
		}
		if want := "2026-06-29T06:07:00Z"; got != want {
			t.Fatalf("normalizePollTimestamp() = %q, want %q", got, want)
		}
	})

	t.Run("RFC3339 offset", func(t *testing.T) {
		got, err := normalizePollTimestamp("2026-06-29T14:07:00+08:00")
		if err != nil {
			t.Fatal(err)
		}
		if want := "2026-06-29T06:07:00Z"; got != want {
			t.Fatalf("normalizePollTimestamp() = %q, want %q", got, want)
		}
	})

	t.Run("empty", func(t *testing.T) {
		got, err := normalizePollTimestamp("")
		if err != nil || got != "" {
			t.Fatalf("normalizePollTimestamp() = %q, %v; want empty value", got, err)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		if _, err := normalizePollTimestamp("not-a-date"); err == nil {
			t.Fatal("normalizePollTimestamp() error = nil, want validation error")
		}
	})
}
