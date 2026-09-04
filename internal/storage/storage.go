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
var ErrStorageFileNotFound = errors.New("Could not find a file in storage")
var ErrStorageDatabaseRequired = errors.New("database connection is required")

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

func (s *Storage) AddUser(name string) (*User, error) {
	id := GenerateUserID()
	u := User{
		ID:   id,
		Name: name,
	}

	_, err := s.db.ExecContext(
		context.Background(),
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

func (s *Storage) GetUser(userId UserID) *User {
	return s.loadUser(userId)
}

func GenerateFileID() FileID {
	return FileID(uuid.New().String())
}

func (s *Storage) GetFile(fileId FileID) *File {
	return s.loadFile(fileId, FileStatusReady)
}

func (s *Storage) Upload(
	ownerId UserID,
	name string,
	fileType FileKind,
	data []byte,
) (*File, error) {
	file, err := s.PreUpload(ownerId, name, fileType, len(data))
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
			_, err := s.UploadPart(file.ID, i, arr)
			errCh <- err
		}(i)
		i += 1
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			_ = s.Delete(file.ID)
			return nil, err
		}
	}

	if err := s.markFileReady(file.ID); err != nil {
		_ = s.Delete(file.ID)
		return nil, err
	}

	return s.GetFile(file.ID), nil
}

func (s *Storage) PreUpload(
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
	}

	_, err := s.db.ExecContext(
		context.Background(),
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
	fileId FileID,
	partPosition int, data []byte,
) (blob *blobs.Blob, err error) {
	blob, err = s.bs.AddOneChunk(data)
	if err != nil {
		return
	}

	err = s.saveUploadedBlob(fileId, partPosition, *blob)

	return
}

func (s *Storage) Delete(fileId FileID) error {
	file := s.loadFile(fileId, "")

	if file == nil {
		return ErrStorageFileNotFound
	}

	_, err := s.db.ExecContext(
		context.Background(),
		`DELETE FROM files WHERE id = $1`,
		fileId,
	)
	if err != nil {
		return err
	}

	return nil
}

func (s *Storage) Rename(fileId FileID, newName string) (*File, error) {
	file := s.GetFile(fileId)
	if file == nil {
		return nil, ErrStorageFileNotFound
	}

	_, err := s.db.ExecContext(
		context.Background(),
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

func (s *Storage) UpdateData(fileId FileID, newData []byte) (*File, error) {
	file := s.GetFile(fileId)
	if file == nil {
		return nil, ErrStorageFileNotFound
	}

	newBlobs, err := s.bs.AddData(newData)
	if err != nil {
		return nil, err
	}

	updatedFile := *file
	updatedFile.Blobs = newBlobs
	if err := s.replaceFileBlobs(&updatedFile, len(newData)); err != nil {
		return nil, err
	}

	file.Blobs = newBlobs
	return file, nil
}

func (s *Storage) Read(fileId FileID) (<-chan []byte, <-chan error) {
	file := s.GetFile(fileId)
	if file == nil {
		dataCh := make(chan []byte)
		errCh := make(chan error)
		go func() {
			close(dataCh)
			errCh <- ErrStorageFileNotFound
			close(errCh)
		}()
		return dataCh, errCh
	}
	return s.bs.ReadBlobs(file.Blobs)
}

func (s *Storage) loadUser(userId UserID) *User {
	row := s.db.QueryRowContext(
		context.Background(),
		`SELECT id, name FROM users WHERE id = $1`,
		userId,
	)

	var user User
	if err := row.Scan(&user.ID, &user.Name); err != nil {
		return nil
	}

	return &user
}

func (s *Storage) loadFile(fileId FileID, status FileStatus) *File {
	query := `SELECT id, name, filetype, owner_id, status FROM files WHERE id = $1`
	args := []any{fileId}
	if status != "" {
		query += ` AND status = $2`
		args = append(args, status)
	}

	row := s.db.QueryRowContext(context.Background(), query, args...)

	var file File
	if err := row.Scan(&file.ID, &file.Name, &file.Filetype, &file.OwnerID, &file.Status); err != nil {
		return nil
	}

	rows, err := s.db.QueryContext(
		context.Background(),
		`SELECT blob_hash, blob_size FROM file_blob_view WHERE file_id = $1 ORDER BY position`,
		fileId,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		var blob blobs.Blob
		if err := rows.Scan(&blob.Hash, &blob.Size); err != nil {
			return nil
		}
		file.Blobs = append(file.Blobs, blob)
	}

	if err := rows.Err(); err != nil {
		return nil
	}

	return &file
}

func (s *Storage) replaceFileBlobs(file *File, size int) error {
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(
		context.Background(),
		`UPDATE files SET size = $1 WHERE id = $2`,
		size,
		file.ID,
	)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(
		context.Background(),
		`DELETE FROM file_blobs WHERE file_id = $1`,
		file.ID,
	)
	if err != nil {
		return err
	}

	if err := saveFileBlobs(context.Background(), tx, file); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Storage) saveUploadedBlob(fileId FileID, position int, blob blobs.Blob) error {
	file := s.loadFile(fileId, FileStatusUploading)
	if file == nil {
		return ErrStorageFileNotFound
	}

	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := saveBlob(context.Background(), tx, blob); err != nil {
		return err
	}

	_, err = tx.ExecContext(
		context.Background(),
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

func (s *Storage) markFileReady(fileId FileID) error {
	_, err := s.db.ExecContext(
		context.Background(),
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
