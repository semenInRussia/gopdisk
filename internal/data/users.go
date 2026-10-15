package data

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gopherdisk/internal/validator"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrUserNotFound   = errors.New("cannot find a user")
	ErrDuplicateEmail = errors.New("a user with this email address already exists")
	ErrInvalidToken   = errors.New("invalid or expired token")

	emailRX = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
)

type UserID string

type User struct {
	ID        UserID   `json:"id"`
	Name      string   `json:"name"`
	Email     string   `json:"email"`
	Activated bool     `json:"activated"`
	Password  Password `json:"-"`
}

type UserModel struct {
	db *sql.DB
}

type dbExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func GenerateUserID() UserID {
	return UserID(uuid.New().String())
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (m UserModel) Insert(ctx context.Context, user User) error {
	return insertUser(ctx, m.db, user)
}

func (m UserModel) Register(ctx context.Context, name, email, plaintextPassword string) (*User, *Token, error) {
	user := User{
		ID:    GenerateUserID(),
		Name:  name,
		Email: NormalizeEmail(email),
	}
	if err := user.Password.Set(plaintextPassword); err != nil {
		return nil, nil, err
	}

	token, err := GenerateToken(user.ID, 3*24*time.Hour, ScopeActivation)
	if err != nil {
		return nil, nil, err
	}

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	if err := insertUser(ctx, tx, user); err != nil {
		return nil, nil, err
	}
	if err := insertToken(ctx, tx, *token); err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}

	return &user, token, nil
}

func (m UserModel) Activate(ctx context.Context, tokenPlaintext string) (*User, error) {
	tokenHash := sha256.Sum256([]byte(tokenPlaintext))

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx, `
		SELECT u.id, u.name, u.email, u.password_hash, u.activated
		FROM users u
		INNER JOIN tokens t ON t.user_id = u.id
		WHERE t.hash = $1 AND t.scope = $2 AND t.expiry > now()
		FOR UPDATE OF u`, tokenHash[:], ScopeActivation)

	user, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidToken
		}
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE users SET activated = TRUE WHERE id = $1`, user.ID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tokens WHERE scope = $1 AND user_id = $2`, ScopeActivation, user.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	user.Activated = true
	return user, nil
}

func insertUser(ctx context.Context, db dbExecutor, user User) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO users (id, name, email, password_hash, activated)
		VALUES ($1, $2, $3, $4, $5)`,
		user.ID, user.Name, user.Email, user.Password.Hash(), user.Activated,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_unique_idx" {
			return ErrDuplicateEmail
		}
		return err
	}
	return nil
}

// New creates an internal user with an unusable password. API registrations
// must use UserModel.Register so that an activation token is also created.
func (m UserModel) New(ctx context.Context, name string) (*User, error) {
	id := GenerateUserID()
	u := User{
		ID:    id,
		Name:  name,
		Email: fmt.Sprintf("legacy-%s@invalid.local", id),
	}
	u.Password.setHash([]byte("!"))

	if err := m.Insert(ctx, u); err != nil {
		return nil, err
	}
	return &u, nil
}

func (m UserModel) Get(ctx context.Context, userID UserID) (*User, error) {
	row := m.db.QueryRowContext(ctx, `
		SELECT id, name, email, password_hash, activated
		FROM users
		WHERE id = $1`, userID)

	user, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}

func (m UserModel) GetByEmail(ctx context.Context, email string) (*User, error) {
	row := m.db.QueryRowContext(ctx, `
		SELECT id, name, email, password_hash, activated
		FROM users
		WHERE lower(email) = lower($1)`, NormalizeEmail(email))

	user, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}

func scanUser(row *sql.Row) (*User, error) {
	var user User
	var passwordHash []byte
	if err := row.Scan(&user.ID, &user.Name, &user.Email, &passwordHash, &user.Activated); err != nil {
		return nil, err
	}
	user.Password.setHash(passwordHash)
	return &user, nil
}

func (m UserModel) Delete(ctx context.Context, userID UserID) error {
	res, err := m.db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", userID)
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

func (m UserModel) GetFilesWithStatus(ctx context.Context, userID UserID, status FileStatus) ([]File, error) {
	user, err := m.Get(ctx, userID)
	if err != nil {
		return nil, err
	}

	rows, err := m.db.QueryContext(ctx, `
		SELECT id, name, filetype, owner_id, status, size
		FROM files
		WHERE owner_id = $1 AND status = $2
		ORDER BY name, id`, user.ID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	files := make([]File, 0)
	for rows.Next() {
		var file File
		if err := rows.Scan(&file.ID, &file.Name, &file.Type, &file.OwnerID, &file.Status, &file.Size); err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return files, nil
}

func (m UserModel) GetFiles(ctx context.Context, userID UserID) ([]File, error) {
	return m.GetFilesWithStatus(ctx, userID, FileStatusReady)
}

func (m UserModel) GetUploads(ctx context.Context, userID UserID) ([]File, error) {
	return m.GetFilesWithStatus(ctx, userID, FileStatusUploading)
}

func ValidateUser(v *validator.Validator, user User) {
	ValidateUserName(v, user.Name)
	ValidateEmail(v, user.Email)
}

func ValidateUserName(v *validator.Validator, name string) {
	v.Check("user", strings.TrimSpace(name) != "", "user's name cannot be empty")
	v.Check("user", validator.RuneCount(name) <= 500, "user's name must not be more than 500 characters long")
}

func ValidateEmail(v *validator.Validator, email string) {
	v.Check("email", email != "", "must be provided")
	v.Check("email", validator.RuneCount(email) <= 500, "must not be more than 500 characters long")
	v.Check("email", validator.Matches(email, emailRX), "must be a valid email address")
}

func ValidatePasswordPlaintext(v *validator.Validator, password string) {
	v.Check("password", password != "", "must be provided")
	v.Check("password", len(password) >= 8, "must be at least 8 bytes long")
	v.Check("password", len(password) <= 72, "must not be more than 72 bytes long")
}
