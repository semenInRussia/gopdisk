package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"gopherdisk/internal/blobs"
	"io"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

type FileID string
type FileStatus string
type FileKind string

var (
	ErrFileNotFound = errors.New("cannot find a file")
	ErrInvalidSize  = errors.New("size must not be negative")
	ErrEarlyCommit  = errors.New("cannot commit file before all data is added")
)

const (
	FileStatusUploading FileStatus = "uploading"
	FileStatusReady     FileStatus = "ready"
)

const (
	checkViolationCode             = "23514"
	filesSizeNonNegativeConstraint = "files_size_non_negative"
	blobsSizeNonNegativeConstraint = "blobs_size_non_negative"
)

type File struct {
	ID       FileID
	Name     string
	Filetype FileKind
	OwnerID  UserID
	Status   FileStatus
	Blobs    []blobs.Blob
	Size     int
}

type FileModel struct {
	bs *blobs.BlobStorage
	db *sql.DB
}

func GenerateFileID() FileID { return FileID(uuid.New().String()) }

// Insert persists an already constructed file without changing its fields.
func (s *FileModel) Insert(ctx context.Context, file File) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `INSERT INTO files (id, name, owner_id, filetype, size, status)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		file.ID, file.Name, file.OwnerID, file.Filetype, file.Size, file.Status)
	if err != nil {
		return handleStorageDatabaseError(err)
	}
	if err := saveFileBlobs(ctx, tx, &file); err != nil {
		return err
	}
	return tx.Commit()
}

// New constructs and inserts an empty file that can receive blobs before Commit.
func (s *FileModel) New(ctx context.Context, ownerID UserID, name string, fileType FileKind, size int) (*File, error) {
	file := File{
		ID: GenerateFileID(), Name: name, Filetype: fileType, OwnerID: ownerID,
		Status: FileStatusUploading, Blobs: []blobs.Blob{}, Size: size,
	}
	if err := s.Insert(ctx, file); err != nil {
		return nil, err
	}
	return &file, nil
}

func (s *FileModel) Get(ctx context.Context, fileID FileID) (*File, error) {
	return s.loadFileResult(ctx, fileID, FileStatusReady)
}

// UploadData is a shortcut for New, AddData, and Commit.
func (s *FileModel) UploadData(ctx context.Context, ownerID UserID, name string, fileType FileKind, data []byte) (*File, error) {
	file, err := s.New(ctx, ownerID, name, fileType, len(data))
	if err != nil {
		return nil, err
	}
	if _, err := s.AddData(ctx, file.ID, data); err != nil {
		_ = s.Delete(context.WithoutCancel(ctx), file.ID)
		return nil, err
	}
	if err := s.Commit(ctx, file.ID); err != nil {
		_ = s.Delete(context.WithoutCancel(ctx), file.ID)
		return nil, err
	}
	return s.Get(ctx, file.ID)
}

// UploadReader is a shortcut for New, AddReader, and Commit.
func (s *FileModel) UploadReader(ctx context.Context, ownerID UserID, name string, fileType FileKind, reader io.Reader, size int) (*File, error) {
	file, err := s.New(ctx, ownerID, name, fileType, size)
	if err != nil {
		return nil, err
	}
	if _, err := s.AddReader(ctx, file.ID, reader); err != nil {
		_ = s.Delete(context.WithoutCancel(ctx), file.ID)
		return nil, err
	}
	if err := s.Commit(ctx, file.ID); err != nil {
		_ = s.Delete(context.WithoutCancel(ctx), file.ID)
		return nil, err
	}
	return s.Get(ctx, file.ID)
}

// AddData stores data as one independent sequence of blobs and appends it to file.
func (s *FileModel) AddData(ctx context.Context, fileID FileID, data []byte) ([]blobs.Blob, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stored, err := s.bs.AddData(data)
	if err != nil {
		return nil, err
	}
	if err := s.appendBlobs(ctx, fileID, stored); err != nil {
		return nil, err
	}
	return stored, nil
}

// AddReader stores reader contents as one independent sequence of blobs and appends it to file.
func (s *FileModel) AddReader(ctx context.Context, fileID FileID, reader io.Reader) ([]blobs.Blob, error) {
	stored, err := s.readAndStoreBlobs(ctx, reader)
	if err != nil {
		return nil, err
	}
	if err := s.appendBlobs(ctx, fileID, stored); err != nil {
		return nil, err
	}
	return stored, nil
}

// AddBlob appends metadata for an already stored physical blob to file.
func (s *FileModel) AddBlob(ctx context.Context, fileID FileID, blob blobs.Blob) error {
	return s.appendBlobs(ctx, fileID, []blobs.Blob{blob})
}

// Commit makes an uploading file visible to regular read and list operations.
func (s *FileModel) Commit(ctx context.Context, fileID FileID) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var declaredSize int
	if err := tx.QueryRowContext(ctx,
		`SELECT size FROM files WHERE id = $1 AND status = $2 FOR UPDATE`,
		fileID, FileStatusUploading).Scan(&declaredSize); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrFileNotFound
		}
		return err
	}

	var storedSize int
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(b.size), 0)
		FROM file_blobs fb
		JOIN blobs b ON b.hash = fb.blob_hash
		WHERE fb.file_id = $1`, fileID).Scan(&storedSize); err != nil {
		return err
	}
	if storedSize != declaredSize {
		return ErrEarlyCommit
	}

	_, err = tx.ExecContext(ctx, `UPDATE files SET status = $1 WHERE id = $2`, FileStatusReady, fileID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *FileModel) Delete(ctx context.Context, fileID FileID) error {
	file, err := s.loadFileResult(ctx, fileID, "")
	if err != nil {
		return err
	}
	if file == nil {
		return ErrFileNotFound
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM files WHERE id = $1`, fileID)
	return err
}

func (s *FileModel) Rename(ctx context.Context, fileID FileID, newName string) (*File, error) {
	file, err := s.Get(ctx, fileID)
	if err != nil {
		return nil, err
	}
	if file == nil {
		return nil, ErrFileNotFound
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE files SET name = $1 WHERE id = $2 AND status = $3`,
		newName, fileID, FileStatusReady)
	if err != nil {
		return nil, err
	}
	file.Name = newName
	return file, nil
}

func (s *FileModel) Read(ctx context.Context, fileID FileID) (<-chan []byte, <-chan error) {
	if err := ctx.Err(); err != nil {
		return closedReadChannels(err)
	}
	file, err := s.loadFileResult(ctx, fileID, FileStatusReady)
	if err != nil {
		return closedReadChannels(err)
	}
	if file == nil {
		return closedReadChannels(ErrFileNotFound)
	}
	return s.bs.ReadBlobsContext(ctx, file.Blobs)
}

func (s *FileModel) List(ctx context.Context, ownerID UserID) ([]File, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM files WHERE owner_id = $1 AND status = $2 ORDER BY name, id`,
		ownerID, FileStatusReady)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fileIDs []FileID
	for rows.Next() {
		var fileID FileID
		if err := rows.Scan(&fileID); err != nil {
			return nil, err
		}
		fileIDs = append(fileIDs, fileID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	files := make([]File, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		file, err := s.loadFileResult(ctx, fileID, FileStatusReady)
		if err != nil {
			return nil, err
		}
		if file != nil {
			files = append(files, *file)
		}
	}
	return files, nil
}

func (s *Storage) IsBelongsToUser(ctx context.Context, fileID FileID, ownerID UserID) (bool, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM files WHERE id = $1 AND owner_id = $2 AND status = $3)`,
		fileID, ownerID, FileStatusReady)
	var exists bool
	if err := row.Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (s *FileModel) loadFileResult(ctx context.Context, fileID FileID, status FileStatus) (*File, error) {
	query := `SELECT id, name, filetype, owner_id, status, size FROM files WHERE id = $1`
	args := []any{fileID}
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

	rows, err := s.db.QueryContext(ctx,
		`SELECT blob_hash, blob_size FROM file_blob_view WHERE file_id = $1 ORDER BY position`, fileID)
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

func (s *FileModel) appendBlobs(ctx context.Context, fileID FileID, appended []blobs.Blob) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists bool
	if err := tx.QueryRowContext(ctx,
		`SELECT TRUE FROM files WHERE id = $1 AND status = $2 FOR UPDATE`,
		fileID, FileStatusUploading).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrFileNotFound
		}
		return err
	}

	var position int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM file_blobs WHERE file_id = $1`, fileID).Scan(&position); err != nil {
		return err
	}

	for _, blob := range appended {
		if err := saveBlob(ctx, tx, blob); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO file_blobs (file_id, blob_hash, position) VALUES ($1, $2, $3)`,
			fileID, blob.Hash, position)
		if err != nil {
			return err
		}
		position++
	}
	return tx.Commit()
}

func (s *FileModel) readAndStoreBlobs(ctx context.Context, reader io.Reader) ([]blobs.Blob, error) {
	var stored []blobs.Blob
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		chunk := make([]byte, blobs.BlobsMaxSize)
		size, err := io.ReadFull(reader, chunk)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, err
		}
		blob, storeErr := s.bs.AddOneChunk(chunk[:size])
		if storeErr != nil {
			return nil, storeErr
		}
		stored = append(stored, *blob)
		if errors.Is(err, io.ErrUnexpectedEOF) {
			break
		}
	}
	return stored, nil
}

func closedReadChannels(err error) (<-chan []byte, <-chan error) {
	dataCh := make(chan []byte)
	errCh := make(chan error, 1)
	close(dataCh)
	if err != nil {
		errCh <- err
	}
	close(errCh)
	return dataCh, errCh
}

func saveFileBlobs(ctx context.Context, tx *sql.Tx, file *File) error {
	for position, blob := range file.Blobs {
		if err := saveBlob(ctx, tx, blob); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO file_blobs (file_id, blob_hash, position) VALUES ($1, $2, $3)`,
			file.ID, blob.Hash, position)
		if err != nil {
			return err
		}
	}
	return nil
}

func saveBlob(ctx context.Context, tx *sql.Tx, blob blobs.Blob) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO blobs (hash, size) VALUES ($1, $2) ON CONFLICT (hash) DO NOTHING`,
		blob.Hash, blob.Size)
	return handleStorageDatabaseError(err)
}

func handleStorageDatabaseError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == checkViolationCode &&
		(pgErr.ConstraintName == filesSizeNonNegativeConstraint || pgErr.ConstraintName == blobsSizeNonNegativeConstraint) {
		return fmt.Errorf("%w: %v", ErrInvalidSize, err)
	}
	return err
}
