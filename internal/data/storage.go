package data

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"time"

	"gopherdisk/internal/blobs"
)

var (
	ErrStorageDatabaseRequired = errors.New("when open storage opened database is expected")
)

type Storage struct {
	bs     *blobs.BlobStorage
	db     *sql.DB
	Users  UserModel
	Files  FileModel
	Tokens TokenModel
}

func MakeStorage(path string, db *sql.DB) (*Storage, error) {
	if db == nil {
		return nil, ErrStorageDatabaseRequired
	}

	bs, err := blobs.MakeBlobStorage(path)
	if err != nil {
		return nil, err
	}

	s := Storage{
		bs:     bs,
		db:     db,
		Files:  FileModel{db: db, bs: bs},
		Users:  UserModel{db: db},
		Tokens: TokenModel{db: db},
	}

	return &s, nil
}

func (s *Storage) RegisterUser(ctx context.Context, name, email, plaintextPassword string) (*User, *Token, error) {
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

	tx, err := s.db.BeginTx(ctx, nil)
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

func (s *Storage) ActivateUser(ctx context.Context, tokenPlaintext string) (*User, error) {
	tokenHash := sha256.Sum256([]byte(tokenPlaintext))

	tx, err := s.db.BeginTx(ctx, nil)
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

func (s *Storage) CollectGarbageBlobs(ctx context.Context) (int, error) {
	query := `
		DELETE FROM blobs b
		WHERE NOT EXISTS (
			SELECT 1
			FROM file_blobs fb
			WHERE fb.blob_hash = b.hash
		)
		returning b.hash, b.size
	`

	rows, err := s.db.QueryContext(ctx, query)

	if err != nil {
		return 0, err
	}

	defer rows.Close()

	cnt := 0

	for rows.Next() {
		var blob blobs.Blob
		err := rows.Scan(&blob.Hash, &blob.Size)
		if err != nil {
			return 0, err
		}
		err = s.bs.DeleteBlob(ctx, blob)
		if err != nil {
			return cnt, err
		}
		cnt += 1
	}

	if err := rows.Err(); err != nil {
		return 0, err
	}

	referencedBlobs, err := s.loadBlobHashes(ctx)
	if err != nil {
		return cnt, err
	}

	storedBlobs, err := s.bs.ListBlobs(ctx)
	if err != nil {
		return cnt, err
	}

	for _, blob := range storedBlobs {
		if referencedBlobs[blob.Hash] {
			continue
		}

		if err := s.bs.DeleteBlob(ctx, blob); err != nil {
			return cnt, err
		}
		cnt++
	}

	return cnt, nil
}

func (s *Storage) loadBlobHashes(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT hash FROM blobs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hashes := make(map[string]bool)

	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return nil, err
		}
		hashes[hash] = true
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return hashes, nil
}
