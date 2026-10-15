package data

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestInsertUserMapsDuplicateEmail(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("cannot create mock database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	user := User{ID: GenerateUserID(), Name: "Kotik", Email: "kotik@example.com"}
	user.Password.setHash([]byte("hash"))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO users (id, name, email, password_hash, activated) VALUES ($1, $2, $3, $4, $5)")).
		WithArgs(user.ID, user.Name, user.Email, user.Password.Hash(), false).
		WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: "users_email_unique_idx"})

	err = (&UserModel{db: db}).Insert(context.Background(), user)
	if !errors.Is(err, ErrDuplicateEmail) {
		t.Fatalf("got error %v, expected %v", err, ErrDuplicateEmail)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet database expectations: %v", err)
	}
}
