package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrationFilesReturnsSortedUpMigrations(t *testing.T) {
	dir := t.TempDir()

	files := []string{
		"000002_second.up.sql",
		"000001_first.up.sql",
		"000001_first.down.sql",
		"notes.txt",
	}

	for _, name := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("-- test"), 0644); err != nil {
			t.Fatalf("cannot write test file: %v", err)
		}
	}

	got, err := migrationFiles(dir)
	if err != nil {
		t.Fatalf("migrationFiles returned error: %v", err)
	}

	want := []string{
		filepath.Join(dir, "000001_first.up.sql"),
		filepath.Join(dir, "000002_second.up.sql"),
	}

	if len(got) != len(want) {
		t.Fatalf("got %d files, expected %d: %v", len(got), len(want), got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %q at index %d, expected %q", got[i], i, want[i])
		}
	}
}

func TestRunRejectsUnsupportedDirection(t *testing.T) {
	err := run([]string{"down"})
	if err == nil {
		t.Fatal("expected error for unsupported migration command")
	}
}

func TestRunRequiresDirection(t *testing.T) {
	err := run(nil)
	if err == nil {
		t.Fatal("expected usage error")
	}
}

func TestMigrationVersion(t *testing.T) {
	got := migrationVersion(filepath.Join("migrations", "000001_create_core_tables.up.sql"))
	want := "000001_create_core_tables"

	if got != want {
		t.Fatalf("got %q, expected %q", got, want)
	}
}
