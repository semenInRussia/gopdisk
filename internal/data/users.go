package data

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

var (
	ErrUserNotFound = errors.New("cannot find an user")
)

type UserID string

type User struct {
	ID   UserID
	Name string
}

type UserModel struct {
	db *sql.DB
}

func GenerateUserID() UserID {
	return UserID(uuid.New().String())
}

func (s *UserModel) Add(ctx context.Context, name string) (*User, error) {
	id := GenerateUserID()
	u := User{
		ID:   id,
		Name: name,
	}

	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO users (id, name) VALUES ($1, $2)`,
		u.ID,
		u.Name,
	)

	if err != nil {
		return nil, err
	}

	return &u, nil
}

func (s *UserModel) Get(ctx context.Context, userId UserID) (*User, error) {
	return s.loadUser(ctx, userId)
}

func (s *UserModel) Delete(ctx context.Context, user UserID) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user)

	if err != nil {
		return err
	}

	cnt, err := res.RowsAffected()

	if err != nil {
		return err
	}

	if cnt == 0 {
		return ErrUserNotFound
	}

	return nil
}

func (s *UserModel) loadUser(ctx context.Context, userId UserID) (*User, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, name FROM users WHERE id = $1`,
		userId,
	)

	var user User
	err := row.Scan(&user.ID, &user.Name)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &user, nil
}
