package data

import (
	"context"
	"database/sql"
	"errors"
	"gopherdisk/internal/blobs"
)

var (
	ErrStorageDatabaseRequired = errors.New("when open storage opened database is expected")
)

type Storage struct {
	bs    *blobs.BlobStorage
	db    *sql.DB
	Users UserModel
	Files FileModel
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
		bs:    bs,
		db:    db,
		Files: FileModel{db: db, bs: bs},
		Users: UserModel{db: db},
	}

	return &s, nil
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
		err = s.bs.DeleteBlob(blob)
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

	storedBlobs, err := s.bs.ListBlobs()
	if err != nil {
		return cnt, err
	}

	for _, blob := range storedBlobs {
		if referencedBlobs[blob.Hash] {
			continue
		}

		if err := s.bs.DeleteBlob(blob); err != nil {
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
