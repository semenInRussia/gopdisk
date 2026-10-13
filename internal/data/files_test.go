package data

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const getUploadQuery = `
	SELECT f.id, f.owner_id, f.name, f.filetype, f.status, f.size, fb.position, b.size
	FROM files f
	LEFT JOIN file_blobs fb ON fb.file_id = f.id
	LEFT JOIN blobs b ON b.hash = fb.blob_hash
	WHERE f.id = $1 AND f.status = $2
	ORDER BY fb.position
`

var getUploadColumns = []string{
	"id", "owner_id", "name", "filetype", "status", "file_size", "position", "blob_size",
}

func newFileModelMock(t *testing.T) (*FileModel, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("cannot create mock database: %v", err)
	}
	t.Cleanup(func() {
		mock.ExpectClose()
		if err := db.Close(); err != nil {
			t.Errorf("cannot close mock database: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet database expectations: %v", err)
		}
	})

	return &FileModel{db: db}, mock
}

func TestFileModelGetUpload(t *testing.T) {
	const (
		fileID  = FileID("32f11d17-b1f5-4fb0-94ca-614903665dff")
		ownerID = UserID("baab6197-3e2d-4969-8492-247bbba900dc")
	)
	expectedQuery := regexp.QuoteMeta(getUploadQuery)

	t.Run("returns metadata and ordered progress", func(t *testing.T) {
		files, mock := newFileModelMock(t)
		mock.ExpectQuery(expectedQuery).
			WithArgs(fileID, FileStatusUploading).
			WillReturnRows(sqlmock.NewRows(getUploadColumns).
				AddRow(fileID, ownerID, "movie.mov", "video/quicktime", FileStatusUploading, int64(40), int64(0), int64(16)).
				AddRow(fileID, ownerID, "movie.mov", "video/quicktime", FileStatusUploading, int64(40), int64(1), int64(16)).
				AddRow(fileID, ownerID, "movie.mov", "video/quicktime", FileStatusUploading, int64(40), int64(2), int64(8)))

		upload, err := files.GetUpload(context.Background(), fileID)
		if err != nil {
			t.Fatalf("GetUpload returned error: %v", err)
		}

		if upload.FileID != fileID || upload.OwnerID != ownerID {
			t.Errorf("got IDs %q/%q, expected %q/%q", upload.FileID, upload.OwnerID, fileID, ownerID)
		}
		if upload.Name != "movie.mov" || upload.FileType != FileKind("video/quicktime") {
			t.Errorf("got metadata %+v, expected movie.mov and video/quicktime", upload)
		}
		if upload.Status != FileStatusUploading || upload.Size != 40 || upload.ReceivedSize != 40 {
			t.Errorf("got status %q, size %d, received size %d; expected uploading, 40, 40", upload.Status, upload.Size, upload.ReceivedSize)
		}

		expectedParts := []UploadPart{
			{Position: 0, Size: 16},
			{Position: 1, Size: 16},
			{Position: 2, Size: 8},
		}
		if len(upload.Parts) != len(expectedParts) {
			t.Fatalf("got %d parts, expected %d", len(upload.Parts), len(expectedParts))
		}
		for i := range expectedParts {
			if upload.Parts[i] != expectedParts[i] {
				t.Errorf("got part %d %+v, expected %+v", i, upload.Parts[i], expectedParts[i])
			}
		}
	})

	t.Run("returns an empty parts slice", func(t *testing.T) {
		files, mock := newFileModelMock(t)
		mock.ExpectQuery(expectedQuery).
			WithArgs(fileID, FileStatusUploading).
			WillReturnRows(sqlmock.NewRows(getUploadColumns).
				AddRow(fileID, ownerID, "empty.txt", "text/plain", FileStatusUploading, int64(0), nil, nil))

		upload, err := files.GetUpload(context.Background(), fileID)
		if err != nil {
			t.Fatalf("GetUpload returned error: %v", err)
		}
		if upload.Parts == nil {
			t.Fatal("got nil parts, expected an empty slice")
		}
		if len(upload.Parts) != 0 || upload.ReceivedSize != 0 {
			t.Errorf("got %d parts and received size %d, expected zero values", len(upload.Parts), upload.ReceivedSize)
		}
	})

	t.Run("returns not found when no uploading file matches", func(t *testing.T) {
		files, mock := newFileModelMock(t)
		mock.ExpectQuery(expectedQuery).
			WithArgs(fileID, FileStatusUploading).
			WillReturnRows(sqlmock.NewRows(getUploadColumns))

		upload, err := files.GetUpload(context.Background(), fileID)
		if !errors.Is(err, ErrFileNotFound) {
			t.Fatalf("got error %v, expected %v", err, ErrFileNotFound)
		}
		if upload != nil {
			t.Fatalf("got upload %+v, expected nil", upload)
		}
	})

	t.Run("returns query errors", func(t *testing.T) {
		files, mock := newFileModelMock(t)
		expectedErr := errors.New("select failed")
		mock.ExpectQuery(expectedQuery).
			WithArgs(fileID, FileStatusUploading).
			WillReturnError(expectedErr)

		upload, err := files.GetUpload(context.Background(), fileID)
		if !errors.Is(err, expectedErr) {
			t.Fatalf("got error %v, expected %v", err, expectedErr)
		}
		if upload != nil {
			t.Fatalf("got upload %+v, expected nil", upload)
		}
	})

	t.Run("returns scan errors", func(t *testing.T) {
		files, mock := newFileModelMock(t)
		mock.ExpectQuery(expectedQuery).
			WithArgs(fileID, FileStatusUploading).
			WillReturnRows(sqlmock.NewRows(getUploadColumns).
				AddRow(fileID, ownerID, "movie.mov", "video/quicktime", FileStatusUploading, "invalid-size", int64(0), int64(16)))

		upload, err := files.GetUpload(context.Background(), fileID)
		if err == nil {
			t.Fatal("got nil error, expected a scan error")
		}
		if upload != nil {
			t.Fatalf("got upload %+v, expected nil", upload)
		}
	})
}
