package data

import (
	"context"
	"database/sql"
	"errors"

	"gopherdisk/internal/validator"

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

func (s *UserModel) Insert(ctx context.Context, user User) error {
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO users (id, name) VALUES ($1, $2)`,
		user.ID,
		user.Name,
	)
	return err
}

func (s *UserModel) New(ctx context.Context, name string) (*User, error) {
	u := User{
		ID:   GenerateUserID(),
		Name: name,
	}

	if err := s.Insert(ctx, u); err != nil {
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

func ValidateUser(v *validator.Validator, user User) {
	ValidateUserName(v, user.Name)
}

func ValidateUserName(v *validator.Validator, userName string) {
	v.Check("user", userName != "", "user's name cannot be empty")
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
