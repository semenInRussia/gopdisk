package storage

import (
	"context"
	"database/sql"
	"gopherdisk/internal/blobs"
)

var MaxPartSize = blobs.BlobsMaxSize

type Storage struct {
	bs blobs.BlobStorage
	db *sql.DB
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
		bs: *bs,
		db: db,
	}

	return &s, nil
}

func (s *Storage) FileBelongsToUser(ctx context.Context, fileId FileID, ownerId UserID) (bool, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT EXISTS (SELECT 1 FROM files WHERE id = $1 AND owner_id = $2 AND status = $3)`,
		fileId,
		ownerId,
		FileStatusReady,
	)

	var exists bool
	if err := row.Scan(&exists); err != nil {
		return false, err
	}

	return exists, nil
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

	return cnt, nil
}
