package storage

import (
	"context"

	"github.com/google/uuid"
)

type UserID string

type User struct {
	ID   UserID
	Name string
}

func GenerateUserID() UserID {
	return UserID(uuid.New().String())
}

func (s *Storage) AddUser(ctx context.Context, name string) (*User, error) {
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

func (s *Storage) GetUser(ctx context.Context, userId UserID) *User {
	return s.loadUser(ctx, userId)
}

func (s *Storage) DeleteUser(ctx context.Context, user UserID) error {
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

func (s *Storage) loadUser(ctx context.Context, userId UserID) *User {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, name FROM users WHERE id = $1`,
		userId,
	)

	var user User
	if err := row.Scan(&user.ID, &user.Name); err != nil {
		return nil
	}

	return &user
}
