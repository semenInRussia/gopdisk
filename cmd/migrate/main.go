package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopherdisk/internal/database"
)

const migrationsDir = "migrations"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: go run ./cmd/migrate up|baseline")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := database.Open(ctx, database.DefaultURL())
	if err != nil {
		return err
	}
	defer db.Close()

	files, err := migrationFiles(migrationsDir)
	if err != nil {
		return err
	}

	if err := ensureSchemaMigrations(ctx, db); err != nil {
		return err
	}

	switch args[0] {
	case "up":
		return runUp(ctx, db, files)
	case "baseline":
		return baseline(ctx, db, files)
	default:
		return fmt.Errorf("unsupported migration command %q", args[0])
	}
}

func runUp(ctx context.Context, db *sql.DB, files []string) error {
	applied, err := appliedMigrations(ctx, db)
	if err != nil {
		return err
	}

	for _, filename := range files {
		version := migrationVersion(filename)
		if applied[version] {
			fmt.Printf("skipped %s\n", filename)
			continue
		}

		if err := applyMigration(ctx, db, filename, version); err != nil {
			return fmt.Errorf("%s: %w", filename, err)
		}
		fmt.Printf("applied %s\n", filename)
	}

	return nil
}

func baseline(ctx context.Context, db *sql.DB, files []string) error {
	for _, filename := range files {
		version := migrationVersion(filename)
		if err := markMigrationApplied(ctx, db, version); err != nil {
			return fmt.Errorf("%s: %w", filename, err)
		}
		fmt.Printf("baselined %s\n", filename)
	}

	return nil
}

func migrationFiles(dir string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		return nil, err
	}

	sort.Strings(matches)
	return matches, nil
}

func migrationVersion(filename string) string {
	base := filepath.Base(filename)
	return base[:len(base)-len(".up.sql")]
}

func ensureSchemaMigrations(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	return err
}

func appliedMigrations(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return applied, nil
}

func applyMigration(ctx context.Context, db *sql.DB, filename string, version string) error {
	query, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, string(query)); err != nil {
		return err
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO schema_migrations (version) VALUES ($1)`,
		version,
	); err != nil {
		return err
	}

	return tx.Commit()
}

func markMigrationApplied(ctx context.Context, db *sql.DB, version string) error {
	_, err := db.ExecContext(
		ctx,
		`INSERT INTO schema_migrations (version) VALUES ($1)
		ON CONFLICT (version) DO NOTHING`,
		version,
	)
	return err
}
