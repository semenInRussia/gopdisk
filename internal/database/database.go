package database

import (
	"context"
	"database/sql"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	EnvDatabaseURL = "DATABASE_URL"
	defaultURL     = "postgres://gopherdisk:gopherdisk@localhost:5432/gopherdisk?sslmode=disable"
)

func DefaultURL() string {
	if databaseURL := os.Getenv(EnvDatabaseURL); databaseURL != "" {
		return databaseURL
	}

	return defaultURL
}

func Open(ctx context.Context, databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	err = Ping(ctx, db)

	if err != nil {
		closeErr := db.Close()
		if closeErr != nil {
			return nil, closeErr
		}
		return nil, err
	}

	return db, nil
}

func Ping(ctx context.Context, db *sql.DB) error {
	return db.PingContext(ctx)
}
