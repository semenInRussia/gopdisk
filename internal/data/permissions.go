package data

import (
	"context"
	"database/sql"
	"errors"

	"gopherdisk/internal/validator"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	PermissionRead   = "files:read"
	PermissionWrite  = "files:write"
	PermissionDelete = "files:delete"
)

type FilePermission struct {
	FileID FileID `json:"file_id"`
	UserID UserID `json:"user_id"`
	Code   string `json:"code"`
}

type PermissionsModel struct {
	db *sql.DB
}

// HasForFile grants owners access without explicit permission rows.
func (m PermissionsModel) HasForFile(ctx context.Context, fileID FileID, userID UserID, code string) (bool, error) {
	var allowed bool
	err := m.db.QueryRowContext(ctx, `
		SELECT f.owner_id = $2 OR EXISTS (
			SELECT 1 FROM file_permissions p
			WHERE p.file_id = f.id AND p.user_id = $2 AND p.code = $3
		)
		FROM files f WHERE f.id = $1`, fileID, userID, code).Scan(&allowed)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrFileNotFound
	}
	return allowed, err
}

func ValidatePermissionCodes(v *validator.Validator, codes []string) {
	v.Check("codes", codes != nil, "must be provided as an array")
	seen := make(map[string]bool)
	for _, code := range codes {
		v.Check("codes", code == PermissionRead || code == PermissionWrite || code == PermissionDelete, "must only contain files:read, files:write or files:delete")
		v.Check("codes", !seen[code], "must not contain duplicates")
		seen[code] = true
	}
}

func (m PermissionsModel) GetAllForFile(ctx context.Context, fileID FileID) ([]FilePermission, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT file_id, user_id, code FROM file_permissions
		WHERE file_id = $1 ORDER BY user_id, code`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	permissions := make([]FilePermission, 0)
	for rows.Next() {
		var permission FilePermission
		if err := rows.Scan(&permission.FileID, &permission.UserID, &permission.Code); err != nil {
			return nil, err
		}
		permissions = append(permissions, permission)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return permissions, nil
}

// SetForUser replaces a user's permissions atomically. An empty set revokes them.
func (m PermissionsModel) SetForUser(ctx context.Context, fileID FileID, userID UserID, codes []string) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Serialize changes for this file, including concurrent replacements of an empty set.
	var lockedID FileID
	err = tx.QueryRowContext(ctx, `SELECT id FROM files WHERE id = $1 FOR UPDATE`, fileID).Scan(&lockedID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrFileNotFound
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM file_permissions WHERE file_id = $1 AND user_id = $2`, fileID, userID); err != nil {
		return err
	}
	for _, code := range codes {
		_, err := tx.ExecContext(ctx, `INSERT INTO file_permissions (file_id, user_id, code) VALUES ($1, $2, $3)`, fileID, userID, code)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" && pgErr.ConstraintName == "file_permissions_user_id_fkey" {
				return ErrUserNotFound
			}
			return err
		}
	}
	return tx.Commit()
}

func (m PermissionsModel) DeleteForUser(ctx context.Context, fileID FileID, userID UserID) error {
	return m.SetForUser(ctx, fileID, userID, nil)
}
