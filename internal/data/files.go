package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"gopherdisk/internal/blobs"
	"gopherdisk/internal/validator"
	"io"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

type FileID string
type FileStatus string
type FileType string

var (
	ErrBlobNotFound    = errors.New("cannot find a blob")
	ErrEarlyComplete   = errors.New("cannot complete file before all data is added")
	ErrFileNotFound    = errors.New("cannot find a file")
	ErrInvalidPosition = errors.New("position must not be negative")
	ErrInvalidSize     = errors.New("size must not be negative")
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
	ID      FileID     `json:"id"`
	Name    string     `json:"name"`
	Type    FileType   `json:"type"`
	OwnerID UserID     `json:"owner_id"`
	Status  FileStatus `json:"status"`
	Size    int64      `json:"size"`
}

type UploadPart struct {
	Position int   `json:"position"`
	Size     int64 `json:"size"`
}

type Upload struct {
	FileID       FileID       `json:"file_id"`
	OwnerID      UserID       `json:"owner_id"`
	Name         string       `json:"name"`
	Type         FileType     `json:"type"`
	Status       FileStatus   `json:"status"`
	Size         int64        `json:"size"`
	ReceivedSize int64        `json:"received_size"`
	Parts        []UploadPart `json:"parts"`
}

type FileModel struct {
	bs *blobs.BlobStorage
	db *sql.DB
}

func ValidateFile(v *validator.Validator, file File) {
	ValidateFileOwnerID(v, file.OwnerID)
	ValidateFileName(v, file.Name)
	ValidateFileType(v, file.Type)
	v.Check("size", file.Size >= 0, "must not be negative")
}

func ValidateFileOwnerID(v *validator.Validator, ownerID UserID) {
	value := string(ownerID)
	v.Check("owner_id", value != "", "must be provided")
	v.Check("owner_id", uuid.Validate(value) == nil, "must be a valid UUID")
}

func ValidateFileName(v *validator.Validator, name string) {
	v.Check("name", strings.TrimSpace(name) != "", "must be provided")
	v.Check("name", validator.RuneCount(name) <= 500, "must not be more than 500 characters long")
}

func ValidateFileType(v *validator.Validator, fileType FileType) {
	value := string(fileType)
	v.Check("type", strings.TrimSpace(value) != "", "must be provided")
	v.Check("type", validator.RuneCount(value) <= 255, "must not be more than 255 characters long")
}

func GenerateFileID() FileID { return FileID(uuid.New().String()) }

func (s *FileModel) Insert(ctx context.Context, file *File) error {
	file.Status = FileStatusUploading
	return s.InsertWithStatus(ctx, file)
}

// Insert persists an already constructed file without changing its fields.
func (s *FileModel) InsertWithStatus(ctx context.Context, file *File) error {
	query := `
		INSERT INTO files (id, name, owner_id, filetype, size, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`

	args := []any{GenerateFileID(), file.Name, file.OwnerID, file.Type, file.Size, file.Status}

	err := s.db.QueryRowContext(ctx, query, args...).Scan(&file.ID)

	return handleStorageDatabaseError(err)
}

// New constructs and inserts an empty file that can receive blobs before Complete.
func (s *FileModel) New(ctx context.Context, ownerID UserID, name string, fileType FileType, size int64) (*File, error) {
	file := File{
		ID: GenerateFileID(), Name: name, Type: fileType, OwnerID: ownerID, Size: size,
	}
	if err := s.Insert(ctx, &file); err != nil {
		return nil, err
	}
	return &file, nil
}

func (s *FileModel) Get(ctx context.Context, fileID FileID) (*File, error) {
	return s.loadFileResult(ctx, fileID, FileStatusReady)
}

// GetUpload returns a consistent snapshot of an unfinished upload and its parts.
func (s *FileModel) GetUpload(ctx context.Context, fileID FileID) (*Upload, error) {
	query := `
		SELECT f.id, f.owner_id, f.name, f.filetype, f.status, f.size, fb.position, b.size
		FROM files f
		LEFT JOIN file_blobs fb ON fb.file_id = f.id
		LEFT JOIN blobs b ON b.hash = fb.blob_hash
		WHERE f.id = $1 AND f.status = $2
		ORDER BY fb.position
	`

	args := []any{fileID, FileStatusUploading}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	upload := Upload{Parts: make([]UploadPart, 0)}
	found := false
	for rows.Next() {
		found = true

		var position sql.NullInt64
		var partSize sql.NullInt64
		if err := rows.Scan(
			&upload.FileID,
			&upload.OwnerID,
			&upload.Name,
			&upload.Type,
			&upload.Status,
			&upload.Size,
			&position,
			&partSize,
		); err != nil {
			return nil, err
		}

		if position.Valid && partSize.Valid {
			upload.Parts = append(upload.Parts, UploadPart{
				Position: int(position.Int64),
				Size:     partSize.Int64,
			})
			upload.ReceivedSize += partSize.Int64
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrFileNotFound
	}

	return &upload, nil
}

func (s *FileModel) GetOneBlob(ctx context.Context, fileID FileID, pos int) (*blobs.Blob, error) {
	query := `
		SELECT b.hash, b.size
		FROM files f
		LEFT JOIN file_blobs fb ON fb.file_id = f.id
		LEFT JOIN blobs b ON b.hash = fb.blob_hash
		WHERE f.id = $1 and fb.position = $2
	`

	args := []any{fileID, pos}

	var blob blobs.Blob
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&blob.Hash, &blob.Size)

	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrBlobNotFound
		default:
			return nil, err
		}
	}

	return &blob, nil
}

// GetBlobs returns file blobs in their logical order.
func (s *FileModel) GetBlobs(ctx context.Context, fileID FileID) ([]blobs.Blob, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT b.hash, b.size
		FROM files f
		LEFT JOIN file_blobs fb ON fb.file_id = f.id
		LEFT JOIN blobs b ON b.hash = fb.blob_hash
		WHERE f.id = $1
		ORDER BY fb.position`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fileBlobs := make([]blobs.Blob, 0)
	found := false
	for rows.Next() {
		found = true
		var hash sql.NullString
		var size sql.NullInt64
		if err := rows.Scan(&hash, &size); err != nil {
			return nil, err
		}
		if hash.Valid {
			fileBlobs = append(fileBlobs, blobs.Blob{Hash: hash.String, Size: size.Int64})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrFileNotFound
	}
	return fileBlobs, nil
}

// UploadData is a shortcut for New, AddDataBack, and Complete.
func (s *FileModel) UploadData(ctx context.Context, ownerID UserID, name string, fileType FileType, data []byte) (*File, error) {
	file, err := s.New(ctx, ownerID, name, fileType, int64(len(data)))
	if err != nil {
		return nil, err
	}
	if _, err := s.AddDataBack(ctx, file.ID, data); err != nil {
		_ = s.Delete(context.WithoutCancel(ctx), file.ID)
		return nil, err
	}
	if err := s.Complete(ctx, file.ID); err != nil {
		_ = s.Delete(context.WithoutCancel(ctx), file.ID)
		return nil, err
	}
	return s.Get(ctx, file.ID)
}

// UploadReader is a shortcut for New, AddReaderBack, and Complete.
func (s *FileModel) UploadReader(ctx context.Context, ownerID UserID, name string, fileType FileType, reader io.Reader, size int64) (*File, error) {
	file, err := s.New(ctx, ownerID, name, fileType, size)
	if err != nil {
		return nil, err
	}
	if _, err := s.AddReaderBack(ctx, file.ID, reader); err != nil {
		_ = s.Delete(context.WithoutCancel(ctx), file.ID)
		return nil, err
	}
	if err := s.Complete(ctx, file.ID); err != nil {
		_ = s.Delete(context.WithoutCancel(ctx), file.ID)
		return nil, err
	}
	return s.Get(ctx, file.ID)
}

// AddDataBack stores data as one independent sequence of blobs and appends it to file.
func (s *FileModel) AddDataBack(ctx context.Context, fileID FileID, data []byte) ([]blobs.Blob, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	type job struct {
		position int
		chunk    []byte
	}

	const nWorkers = 4

	jobs := make(chan job, nWorkers)
	chunks := s.splitIntoBlobChunks(data)
	stored := make([]blobs.Blob, len(chunks))

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var errOnce sync.Once
	var firstErr error
	fail := func(err error) {
		if err == nil {
			return
		}
		errOnce.Do(func() {
			firstErr = err
			cancel()
		})
	}

	var wg sync.WaitGroup
	for range nWorkers {
		wg.Go(func() {
			for job := range jobs {
				blob, err := s.bs.AddData(workerCtx, job.chunk)
				if err != nil {
					fail(err)
					return
				}
				stored[job.position] = *blob
			}
		})
	}

loop:
	for position, chunk := range chunks {
		select {
		case jobs <- job{position: position, chunk: chunk}:
		case <-workerCtx.Done():
			fail(workerCtx.Err())
			break loop
		}
	}
	close(jobs)
	wg.Wait()

	if err := workerCtx.Err(); err != nil {
		fail(err)
	}
	if firstErr != nil {
		return nil, firstErr
	}
	if err := s.appendBlobs(ctx, fileID, stored); err != nil {
		return nil, err
	}
	return stored, nil
}

// AddReaderBack stores reader contents as an ordered sequence of blobs and appends it to file.
func (s *FileModel) AddReaderBack(ctx context.Context, fileID FileID, reader io.Reader) ([]blobs.Blob, error) {
	stored := make([]blobs.Blob, 0)
	for {
		blob, err := s.bs.AddReader(ctx, reader)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		stored = append(stored, *blob)
	}
	if err := s.appendBlobs(ctx, fileID, stored); err != nil {
		return nil, err
	}
	return stored, nil
}

func (s *FileModel) splitIntoBlobChunks(data []byte) [][]byte {
	chunks := make([][]byte, 0, s.blobCountForSize(int64(len(data))))
	for left := int64(0); left < int64(len(data)); {
		right := min(left+blobs.BlobsMaxSize, int64(len(data)))
		chunks = append(chunks, data[left:right])
		left = right
	}
	return chunks
}

func (s *FileModel) blobCountForSize(size int64) int {
	return int((size + blobs.BlobsMaxSize - 1) / blobs.BlobsMaxSize)
}

// AddBlobBack appends metadata for an already stored physical blob to file.
func (s *FileModel) AddBlobBack(ctx context.Context, fileID FileID, blob blobs.Blob) error {
	return s.appendBlobs(ctx, fileID, []blobs.Blob{blob})
}

// AddData stores data as one blob at position, replacing any blob already there.
func (s *FileModel) AddData(ctx context.Context, fileID FileID, position int, data []byte) (*blobs.Blob, error) {
	if position < 0 {
		return nil, ErrInvalidPosition
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	stored, err := s.bs.AddData(ctx, data)
	if err != nil {
		return nil, err
	}
	if err := s.putBlob(ctx, fileID, position, *stored); err != nil {
		return nil, err
	}
	return stored, nil
}

// AddReader stores at most one blob from reader at position, replacing any blob
// already there and leaving data beyond BlobsMaxSize unread.
func (s *FileModel) AddReader(ctx context.Context, fileID FileID, position int, reader io.Reader) (*blobs.Blob, error) {
	if position < 0 {
		return nil, ErrInvalidPosition
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	stored, err := s.bs.AddReader(ctx, reader)
	if err != nil {
		return nil, err
	}
	if err := s.putBlob(ctx, fileID, position, *stored); err != nil {
		return nil, err
	}
	return stored, nil
}

// AddBlob associates an already stored physical blob with position, replacing any blob already there.
func (s *FileModel) AddBlob(ctx context.Context, fileID FileID, position int, blob blobs.Blob) error {
	if position < 0 {
		return ErrInvalidPosition
	}
	return s.putBlob(ctx, fileID, position, blob)
}

// RemoveBlob removes the blob association at position from an unfinished upload.
// The physical blob remains available for reference-aware garbage collection.
func (s *FileModel) RemoveBlob(ctx context.Context, fileID FileID, position int) error {
	if position < 0 {
		return ErrInvalidPosition
	}
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

	result, err := tx.ExecContext(ctx,
		`DELETE FROM file_blobs WHERE file_id = $1 AND position = $2`,
		fileID, position)
	if err != nil {
		return err
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if removed == 0 {
		return ErrBlobNotFound
	}

	return tx.Commit()
}

// Complete validates an upload and makes it visible to regular read and list operations.
func (s *FileModel) Complete(ctx context.Context, fileID FileID) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var declaredSize int64
	if err := tx.QueryRowContext(ctx,
		`SELECT size FROM files WHERE id = $1 AND status = $2 FOR UPDATE`,
		fileID, FileStatusUploading).Scan(&declaredSize); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrFileNotFound
		}
		return err
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT fb.position, b.size
		FROM file_blobs fb
		JOIN blobs b ON b.hash = fb.blob_hash
		WHERE fb.file_id = $1
		ORDER BY fb.position`, fileID)
	if err != nil {
		return err
	}

	parts := make([]UploadPart, 0)
	for rows.Next() {
		var part UploadPart
		if err := rows.Scan(&part.Position, &part.Size); err != nil {
			rows.Close()
			return err
		}
		parts = append(parts, part)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	if err := validateUploadParts(declaredSize, parts); err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `UPDATE files SET status = $1 WHERE id = $2`, FileStatusReady, fileID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func validateUploadParts(declaredSize int64, parts []UploadPart) error {
	var storedSize int64
	for i, part := range parts {
		if part.Position != i {
			return fmt.Errorf("%w: expected part at position %d, got %d", ErrEarlyComplete, i, part.Position)
		}
		if i < len(parts)-1 && part.Size != blobs.BlobsMaxSize {
			return fmt.Errorf("%w: part %d has size %d, expected %d", ErrEarlyComplete, part.Position, part.Size, blobs.BlobsMaxSize)
		}
		if i == len(parts)-1 && part.Size > blobs.BlobsMaxSize {
			return fmt.Errorf("%w: final part has size %d, maximum is %d", ErrEarlyComplete, part.Size, blobs.BlobsMaxSize)
		}
		storedSize += part.Size
	}
	if storedSize != declaredSize {
		return fmt.Errorf("%w: received %d of %d bytes", ErrEarlyComplete, storedSize, declaredSize)
	}
	return nil
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

// CancelUpload removes an unfinished upload and its blob associations.
// Physical blobs remain available for reference-aware garbage collection.
func (s *FileModel) CancelUpload(ctx context.Context, fileID FileID) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	result, err := s.db.ExecContext(ctx,
		`DELETE FROM files WHERE id = $1 AND status = $2`,
		fileID, FileStatusUploading)
	if err != nil {
		return err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if deleted == 0 {
		return ErrFileNotFound
	}
	return nil
}

func (s *FileModel) Rename(ctx context.Context, fileID FileID, newName string) (*File, error) {
	file, err := s.Get(ctx, fileID)
	if err != nil {
		return nil, err
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
	fileBlobs, err := s.GetBlobs(ctx, fileID)
	if err != nil {
		return closedReadChannels(err)
	}
	return s.bs.ReadBlobs(ctx, fileBlobs)
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
	err := row.Scan(&file.ID, &file.Name, &file.Type, &file.OwnerID, &file.Status, &file.Size)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrFileNotFound
		default:
			return nil, err
		}
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
		`SELECT COALESCE(MAX(position), -1) + 1 FROM file_blobs WHERE file_id = $1`, fileID).Scan(&position); err != nil {
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

func (s *FileModel) putBlob(ctx context.Context, fileID FileID, position int, blob blobs.Blob) error {
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

	if err := saveBlob(ctx, tx, blob); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO file_blobs (file_id, blob_hash, position)
		VALUES ($1, $2, $3)
		ON CONFLICT (file_id, position)
		DO UPDATE SET blob_hash = EXCLUDED.blob_hash`, fileID, blob.Hash, position)
	if err != nil {
		return err
	}
	return tx.Commit()
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
