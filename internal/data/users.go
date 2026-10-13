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
	ID   UserID `json:"id"`
	Name string `json:"name"`
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
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, name FROM users WHERE id = $1`,
		userId,
	)

	var user User
	err := row.Scan(&user.ID, &user.Name)

	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrUserNotFound
		default:
			return nil, err
		}
	}

	return &user, nil
}

func (s *UserModel) Delete(ctx context.Context, user UserID) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user)

	cnt, err := res.RowsAffected()

	if err != nil {
		return err
	}

	if cnt == 0 {
		return ErrUserNotFound
	}

	return nil
}

func (s *UserModel) GetFilesWithStatus(ctx context.Context, userID UserID, status FileStatus) ([]File, error) {
	user, err := s.Get(ctx, userID)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT id, name, filetype, owner_id, status, size
		FROM files
		WHERE owner_id = $1 AND status = $2
		ORDER BY name, id`

	args := []any{user.ID, status}

	rows, err := s.db.QueryContext(ctx, query, args...)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	files := make([]File, 0)

	for rows.Next() {
		var file File
		err := rows.Scan(&file.ID, &file.Name, &file.Type, &file.OwnerID, &file.Status, &file.Size)

		if err != nil {
			return nil, err
		}

		files = append(files, file)
	}

	err = rows.Err()

	if err != nil {
		return nil, err
	}

	return files, nil
}

func (s *UserModel) GetFiles(ctx context.Context, userID UserID) ([]File, error) {
	return s.GetFilesWithStatus(ctx, userID, FileStatusReady)
}

func (s *UserModel) GetUploads(ctx context.Context, userID UserID) ([]File, error) {
	return s.GetFilesWithStatus(ctx, userID, FileStatusUploading)
}

func ValidateUser(v *validator.Validator, user User) {
	ValidateUserName(v, user.Name)
}

func ValidateUserName(v *validator.Validator, userName string) {
	v.Check("user", userName != "", "user's name cannot be empty")
}
