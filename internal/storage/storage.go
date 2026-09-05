package storage

import (
	"context"
	"database/sql"
	"errors"
	"gopherdisk/internal/blobs"
	"sync"

	"github.com/google/uuid"
)

var MaxPartSize = blobs.BlobsMaxSize
var ErrEarlyFinishUpload = errors.New("Finishing uploading before each parst was upload")
var ErrStorageFileNotFound = errors.New("Could not find a file in storage")
var ErrStorageDatabaseRequired = errors.New("Database connection is required")
var ErrIndexOutOfBounds = errors.New("Index out of bounds")

type FileID string
type UserID string
type FileKind string
type FileStatus string

const (
	FileStatusUploading FileStatus = "uploading"
	FileStatusReady     FileStatus = "ready"
)

type User struct {
	ID   UserID
	Name string
}

type File struct {
	ID       FileID
	Name     string
	Filetype FileKind
	OwnerID  UserID
	Status   FileStatus
	Blobs    []blobs.Blob
	Size     int
}

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

func GenerateUserID() UserID {
	return UserID(uuid.New().String())
}

func (s *Storage) GetUser(ctx context.Context, userId UserID) *User {
	return s.loadUser(ctx, userId)
}

func GenerateFileID() FileID {
	return FileID(uuid.New().String())
}

func (s *Storage) GetFile(ctx context.Context, fileId FileID) *File {
	return s.loadFile(ctx, fileId, FileStatusReady)
}

func (s *Storage) Upload(
	ctx context.Context,
	ownerId UserID,
	name string,
	fileType FileKind,
	data []byte,
) (*File, error) {
	file, err := s.PreUpload(ctx, ownerId, name, fileType, len(data))
	if err != nil {
		return nil, err
	}

	i := 0
	var wg sync.WaitGroup
	errCh := make(chan error, blobs.ComputeChunksAmount(len(data)))

	for arr := range blobs.SplitDataOnChunks(data) {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.UploadPart(ctx, file.ID, i, arr)
			errCh <- err
		}(i)
		i += 1
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			_ = s.Delete(context.WithoutCancel(ctx), file.ID)
			return nil, err
		}
	}

	if err := s.FinishUpload(ctx, file.ID); err != nil {
		_ = s.Delete(context.WithoutCancel(ctx), file.ID)
		return nil, err
	}

	return s.GetFile(ctx, file.ID), nil
}

func (s *Storage) PreUpload(
	ctx context.Context,
	ownerId UserID,
	name string,
	fileType FileKind,
	size int,
) (*File, error) {
	nChunks := blobs.ComputeChunksAmount(size)

	file := File{
		ID:       GenerateFileID(),
		Name:     name,
		Filetype: fileType,
		OwnerID:  ownerId,
		Status:   FileStatusUploading,
		Blobs:    make([]blobs.Blob, nChunks),
		Size:     size,
	}

	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO files (id, name, owner_id, filetype, size, status)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		file.ID,
		file.Name,
		file.OwnerID,
		file.Filetype,
		size,
		file.Status,
	)
	if err != nil {
		return nil, err
	}

	return &file, nil
}

func (s *Storage) UploadPart(
	ctx context.Context,
	fileId FileID,
	partPosition int, data []byte,
) (blob *blobs.Blob, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	blob, err = s.bs.AddOneChunk(data)
	if err != nil {
		return
	}

	err = s.saveUploadedBlob(ctx, fileId, partPosition, *blob)

	return
}

func (s *Storage) FinishUpload(ctx context.Context, fileId FileID) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	file, err := s.loadFileResult(ctx, fileId, FileStatusUploading)
	if err != nil {
		return err
	}
	if file == nil {
		return ErrStorageFileNotFound
	}

	sz := 0
	for _, b := range file.Blobs {
		sz += b.Size
	}

	if sz != file.Size {
		return ErrEarlyFinishUpload
	}

	return s.markFileReady(ctx, fileId)
}

func (s *Storage) Delete(ctx context.Context, fileId FileID) error {
	file, err := s.loadFileResult(ctx, fileId, "")
	if err != nil {
		return err
	}

	if file == nil {
		return ErrStorageFileNotFound
	}

	_, err = s.db.ExecContext(
		ctx,
		`DELETE FROM files WHERE id = $1`,
		fileId,
	)
	if err != nil {
		return err
	}

	return nil
}

func (s *Storage) Rename(ctx context.Context, fileId FileID, newName string) (*File, error) {
	file := s.GetFile(ctx, fileId)
	if file == nil {
		return nil, ErrStorageFileNotFound
	}

	_, err := s.db.ExecContext(
		ctx,
		`UPDATE files SET name = $1 WHERE id = $2 AND status = $3`,
		newName,
		fileId,
		FileStatusReady,
	)
	if err != nil {
		return nil, err
	}

	file.Name = newName
	return file, nil
}

func (s *Storage) UpdateData(ctx context.Context, fileId FileID, newData []byte) (*File, error) {
	file := s.GetFile(ctx, fileId)
	if file == nil {
		return nil, ErrStorageFileNotFound
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	newBlobs, err := s.bs.AddData(newData)
	if err != nil {
		return nil, err
	}

	updatedFile := *file
	updatedFile.Blobs = newBlobs
	if err := s.replaceFileBlobs(ctx, &updatedFile, len(newData)); err != nil {
		return nil, err
	}

	file.Blobs = newBlobs
	return file, nil
}

func (s *Storage) Read(ctx context.Context, fileId FileID) (<-chan []byte, <-chan error) {
	if err := ctx.Err(); err != nil {
		dataCh := make(chan []byte)
		errCh := make(chan error, 1)
		close(dataCh)
		errCh <- err
		close(errCh)
		return dataCh, errCh
	}

	file := s.GetFile(ctx, fileId)
	if file == nil {
		dataCh := make(chan []byte)
		errCh := make(chan error, 1)
		go func() {
			close(dataCh)
			errCh <- ErrStorageFileNotFound
			close(errCh)
		}()
		return dataCh, errCh
	}
	return s.bs.ReadBlobs(file.Blobs)
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

func (s *Storage) loadFile(ctx context.Context, fileId FileID, status FileStatus) *File {
	file, err := s.loadFileResult(ctx, fileId, status)
	if err != nil {
		return nil
	}
	return file
}

func (s *Storage) loadFileResult(ctx context.Context, fileId FileID, status FileStatus) (*File, error) {
	query := `SELECT id, name, filetype, owner_id, status, size FROM files WHERE id = $1`
	args := []any{fileId}
	if status != "" {
		query += ` AND status = $2`
		args = append(args, status)
	}

	row := s.db.QueryRowContext(ctx, query, args...)

	var file File
	if err := row.Scan(&file.ID, &file.Name, &file.Filetype, &file.OwnerID, &file.Status, &file.Size); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT blob_hash, blob_size FROM file_blob_view WHERE file_id = $1 ORDER BY position`,
		fileId,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var blob blobs.Blob
		if err := rows.Scan(&blob.Hash, &blob.Size); err != nil {
			return nil, err
		}
		file.Blobs = append(file.Blobs, blob)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &file, nil
}

func (s *Storage) replaceFileBlobs(ctx context.Context, file *File, size int) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(
		ctx,
		`UPDATE files SET size = $1 WHERE id = $2`,
		size,
		file.ID,
	)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(
		ctx,
		`DELETE FROM file_blobs WHERE file_id = $1`,
		file.ID,
	)
	if err != nil {
		return err
	}

	if err := saveFileBlobs(ctx, tx, file); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Storage) saveUploadedBlob(ctx context.Context, fileId FileID, position int, blob blobs.Blob) error {
	file, err := s.loadFileResult(ctx, fileId, FileStatusUploading)
	if err != nil {
		return err
	}
	if file == nil {
		return ErrStorageFileNotFound
	}

	chunksAmount, err := s.loadFileChunksAmount(ctx, fileId)
	if err != nil {
		return err
	}
	if position < 0 || position >= chunksAmount {
		return ErrIndexOutOfBounds
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := saveBlob(ctx, tx, blob); err != nil {
		return err
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO file_blobs (file_id, blob_hash, position)
		VALUES ($1, $2, $3)
		ON CONFLICT (file_id, position)
		DO UPDATE SET blob_hash = EXCLUDED.blob_hash`,
		fileId,
		blob.Hash,
		position,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Storage) loadFileChunksAmount(ctx context.Context, fileId FileID) (int, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT size FROM files WHERE id = $1`,
		fileId,
	)

	var size int
	if err := row.Scan(&size); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrStorageFileNotFound
		}
		return 0, err
	}

	return blobs.ComputeChunksAmount(size), nil
}

func (s *Storage) markFileReady(ctx context.Context, fileId FileID) error {
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE files SET status = $1 WHERE id = $2 AND status = $3`,
		FileStatusReady,
		fileId,
		FileStatusUploading,
	)
	return err
}

func saveFileBlobs(ctx context.Context, tx *sql.Tx, file *File) error {
	for position, blob := range file.Blobs {
		if err := saveBlob(ctx, tx, blob); err != nil {
			return err
		}

		_, err := tx.ExecContext(
			ctx,
			`INSERT INTO file_blobs (file_id, blob_hash, position)
			VALUES ($1, $2, $3)`,
			file.ID,
			blob.Hash,
			position,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func saveBlob(ctx context.Context, tx *sql.Tx, blob blobs.Blob) error {
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO blobs (hash, size) VALUES ($1, $2)
		ON CONFLICT (hash) DO NOTHING`,
		blob.Hash,
		blob.Size,
	)
	return err
}
