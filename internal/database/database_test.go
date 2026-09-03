package database

import "testing"

func TestDefaultURLUsesFallback(t *testing.T) {
	t.Setenv(EnvDatabaseURL, "")

	got := DefaultURL()
	want := "postgres://gopherdisk:gopherdisk@localhost:5432/gopherdisk?sslmode=disable"

	if got != want {
		t.Fatalf("got %q, expected %q", got, want)
	}
}

func TestDefaultURLUsesEnvironment(t *testing.T) {
	want := "postgres://user:pass@localhost:5433/test?sslmode=disable"
	t.Setenv(EnvDatabaseURL, want)

	got := DefaultURL()

	if got != want {
		t.Fatalf("got %q, expected %q", got, want)
	}
}
