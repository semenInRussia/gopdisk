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

func TestGetUserByEmail(t *testing.T) {
	const userID = "baab6197-3e2d-4969-8492-247bbba900dc"
	query := regexp.QuoteMeta("SELECT id, name, email, password_hash, activated FROM users WHERE lower(email) = lower($1)")
	columns := []string{"id", "name", "email", "password_hash", "activated"}

	t.Run("returns user using normalized email", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("cannot create mock database: %v", err)
		}
		t.Cleanup(func() { db.Close() })

		passwordHash := []byte("password-hash")
		mock.ExpectQuery(query).
			WithArgs("kotik@example.com").
			WillReturnRows(sqlmock.NewRows(columns).
				AddRow(userID, "Kotik", "kotik@example.com", passwordHash, true))

		user, err := (&UserModel{db: db}).GetByEmail(context.Background(), " KOTIK@Example.COM ")
		if err != nil {
			t.Fatalf("cannot get user by email: %v", err)
		}
		if user.ID != UserID(userID) || user.Name != "Kotik" || user.Email != "kotik@example.com" || !user.Activated {
			t.Fatalf("got unexpected user: %+v", user)
		}
		if string(user.Password.Hash()) != string(passwordHash) {
			t.Fatalf("got password hash %q, expected %q", user.Password.Hash(), passwordHash)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet database expectations: %v", err)
		}
	})

	t.Run("returns not found", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("cannot create mock database: %v", err)
		}
		t.Cleanup(func() { db.Close() })

		mock.ExpectQuery(query).
			WithArgs("missing@example.com").
			WillReturnRows(sqlmock.NewRows(columns))

		user, err := (&UserModel{db: db}).GetByEmail(context.Background(), "missing@example.com")
		if !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("got error %v, expected %v", err, ErrUserNotFound)
		}
		if user != nil {
			t.Fatalf("got user %+v, expected nil", user)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet database expectations: %v", err)
		}
	})
}
