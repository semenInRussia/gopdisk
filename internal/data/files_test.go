package data

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"testing/iotest"

	"gopherdisk/internal/blobs"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
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
		if upload.Name != "movie.mov" || upload.Type != FileType("video/quicktime") {
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

func readAll(t *testing.T, s *Storage, fileID FileID) ([]byte, error) {
	t.Helper()

	dataCh, errCh := s.Files.Read(context.Background(), fileID)

	var data []byte

	for chunk := range dataCh {
		data = append(data, chunk...)
	}

	err, _ := <-errCh

	return data, err
}

func TestInsertFilePersistsConstructedFile(t *testing.T) {
	s := getStorage(t)
	kotik, err := s.Users.New(context.Background(), "Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}
	file := File{
		ID: GenerateFileID(), Name: "Empty", Type: "TXT", OwnerID: kotik.ID,
		Status: FileStatusReady, Size: 0,
	}
	if err := s.Files.InsertWithStatus(context.Background(), &file); err != nil {
		t.Fatalf("cannot insert file: %v", err)
	}
	got, err := s.Files.Get(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("cannot get inserted file: %v", err)
	}
	if got == nil || got.ID != file.ID || got.Status != file.Status || got.Size != file.Size {
		t.Fatalf("got %+v, expected %+v", got, file)
	}
}

func TestGetFileReturnsNotFound(t *testing.T) {
	s := getStorage(t)

	got, err := s.Files.Get(context.Background(), GenerateFileID())
	if !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("got error %v, expected %v", err, ErrFileNotFound)
	}
	if got != nil {
		t.Fatalf("got file %+v, expected nil", got)
	}
}

func TestInsertRejectsNegativeSize(t *testing.T) {
	s := getStorage(t)
	kotik, err := s.Users.New(context.Background(), "Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}
	err = s.Files.InsertWithStatus(context.Background(), &File{
		ID: GenerateFileID(), Name: "Invalid", Type: "TXT", OwnerID: kotik.ID,
		Status: FileStatusUploading, Size: -1,
	})
	if !errors.Is(err, ErrInvalidSize) {
		t.Fatalf("got error %v, expected %v", err, ErrInvalidSize)
	}
}

func TestHandleStorageDatabaseErrorMapsSizeCheckViolation(t *testing.T) {
	err := handleStorageDatabaseError(&pgconn.PgError{
		Code:           checkViolationCode,
		ConstraintName: filesSizeNonNegativeConstraint,
	})

	if !errors.Is(err, ErrInvalidSize) {
		t.Fatalf("got error %v, expected %v", err, ErrInvalidSize)
	}
}

func TestUploadAndRead(t *testing.T) {
	s := getStorage(t)

	kotik, err := s.Users.New(context.Background(), "Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}

	notebookText := []byte("Hi man!")

	file, err := s.Files.UploadData(context.Background(),
		kotik.ID,
		"Notebook",
		"TXT",
		notebookText,
	)
	if err != nil {
		t.Fatalf("cannot upload Notebook: %v", err)
	}

	data, err := readAll(t, s, file.ID)
	if err != nil {
		t.Fatalf("cannot read Notebook: %v", err)
	}

	if !bytes.Equal(data, notebookText) {
		t.Fatalf(
			"got %q, expected %q",
			data,
			notebookText,
		)
	}
}

func TestNewAddDataCommitAndRead(t *testing.T) {
	previousMaxPartSize := blobs.BlobsMaxSize
	blobs.BlobsMaxSize = 4
	t.Cleanup(func() {
		blobs.BlobsMaxSize = previousMaxPartSize
	})

	s := getStorage(t)

	kotik, err := s.Users.New(context.Background(), "Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}

	file, err := s.Files.New(context.Background(), kotik.ID, "Notebook", "TXT", int64(len("hello-world")))
	if err != nil {
		t.Fatalf("cannot create file: %v", err)
	}
	if file.Size != int64(len("hello-world")) || file.Status != FileStatusUploading {
		t.Fatalf("new file has wrong metadata: %+v", file)
	}
	fileBlobs, err := s.Files.GetBlobs(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("cannot get new file blobs: %v", err)
	}
	if len(fileBlobs) != 0 {
		t.Fatalf("new file already has %d blobs", len(fileBlobs))
	}
	for i, part := range [][]byte{[]byte("hell"), []byte("o-world")} {
		if _, err := s.Files.AddDataBack(context.Background(), file.ID, part); err != nil {
			t.Fatalf("cannot add data %d: %v", i, err)
		}
	}
	if err := s.Files.Commit(context.Background(), file.ID); err != nil {
		t.Fatalf("cannot commit file: %v", err)
	}
	got, err := readAll(t, s, file.ID)
	if err != nil {
		t.Fatalf("cannot read finished file: %v", err)
	}

	if !bytes.Equal(got, []byte("hello-world")) {
		t.Fatalf("got %q, expected %q", got, "hello-world")
	}
	loaded, err := s.Files.Get(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("cannot reload file: %v", err)
	}
	fileBlobs, err = s.Files.GetBlobs(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("cannot get committed file blobs: %v", err)
	}
	if loaded.Size != int64(len("hello-world")) || len(fileBlobs) != 3 {
		t.Fatalf("got size %d and %d blobs, expected size 11 and 3 blobs", loaded.Size, len(fileBlobs))
	}
	for i, expectedSize := range []int{4, 4, 3} {
		if fileBlobs[i].Size != int64(expectedSize) {
			t.Fatalf("blob %d has size %d, expected %d", i, fileBlobs[i].Size, expectedSize)
		}
	}
}

func TestCommitRequiresDeclaredSize(t *testing.T) {
	s := getStorage(t)
	user, err := s.Users.New(context.Background(), "Kotik")
	if err != nil {
		t.Fatalf("cannot add user: %v", err)
	}
	file, err := s.Files.New(context.Background(), user.ID, "Notebook", "TXT", 5)
	if err != nil {
		t.Fatalf("cannot create file: %v", err)
	}
	if _, err := s.Files.AddDataBack(context.Background(), file.ID, []byte("four")); err != nil {
		t.Fatalf("cannot add data: %v", err)
	}
	if err := s.Files.Commit(context.Background(), file.ID); !errors.Is(err, ErrEarlyCommit) {
		t.Fatalf("got error %v, expected %v", err, ErrEarlyCommit)
	}
	if _, err := s.Files.AddDataBack(context.Background(), file.ID, []byte("!")); err != nil {
		t.Fatalf("cannot add remaining data: %v", err)
	}
	if err := s.Files.Commit(context.Background(), file.ID); err != nil {
		t.Fatalf("cannot commit complete file: %v", err)
	}
}

func TestUploadReaderAndRead(t *testing.T) {
	previousMaxPartSize := blobs.BlobsMaxSize
	blobs.BlobsMaxSize = 4
	t.Cleanup(func() {
		blobs.BlobsMaxSize = previousMaxPartSize
	})

	s := getStorage(t)
	user, err := s.Users.New(context.Background(), "Reader User")
	if err != nil {
		t.Fatalf("cannot add user: %v", err)
	}

	cases := []struct {
		name string
		data string
	}{
		{name: "empty", data: ""},
		{name: "small", data: "abc"},
		{name: "exact", data: "abcd"},
		{name: "plus-one", data: "abcde"},
		{name: "multiple", data: "abcdefghi"},
	}

	for _, tc := range cases {
		file, err := s.Files.UploadReader(context.Background(), user.ID, tc.name, "TXT", strings.NewReader(tc.data), int64(len(tc.data)))
		if err != nil {
			t.Fatalf("cannot upload %s: %v", tc.name, err)
		}

		got, err := readAll(t, s, file.ID)
		if err != nil {
			t.Fatalf("cannot read %s: %v", tc.name, err)
		}
		if string(got) != tc.data {
			t.Fatalf("got %q, expected %q", got, tc.data)
		}
	}
}

func TestAddReaderBackReturnsReaderErrors(t *testing.T) {
	previousMaxPartSize := blobs.BlobsMaxSize
	blobs.BlobsMaxSize = 4
	t.Cleanup(func() { blobs.BlobsMaxSize = previousMaxPartSize })

	s := getStorage(t)
	user, err := s.Users.New(context.Background(), "Reader error user")
	if err != nil {
		t.Fatalf("cannot add user: %v", err)
	}
	file, err := s.Files.New(context.Background(), user.ID, "reader-error", "TXT", 8)
	if err != nil {
		t.Fatalf("cannot create file: %v", err)
	}

	expectedErr := errors.New("reader failed")
	reader := io.MultiReader(strings.NewReader("abcd"), iotest.ErrReader(expectedErr))
	if _, err := s.Files.AddReaderBack(context.Background(), file.ID, reader); !errors.Is(err, expectedErr) {
		t.Fatalf("AddReaderBack returned %v, expected %v", err, expectedErr)
	}

	fileBlobs, err := s.Files.GetBlobs(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("cannot get file blobs: %v", err)
	}
	if len(fileBlobs) != 0 {
		t.Fatalf("got %d associated blobs after reader error, expected 0", len(fileBlobs))
	}

	entries, err := os.ReadDir(s.bs.Path)
	if err != nil {
		t.Fatalf("cannot inspect blob storage: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".blob-") {
			t.Errorf("temporary blob %q was not cleaned up", entry.Name())
		}
	}
}

func TestAddReaderAndAddBlob(t *testing.T) {
	previousMaxPartSize := blobs.BlobsMaxSize
	blobs.BlobsMaxSize = 4
	t.Cleanup(func() { blobs.BlobsMaxSize = previousMaxPartSize })

	s := getStorage(t)
	user, err := s.Users.New(context.Background(), "Mixed input user")
	if err != nil {
		t.Fatalf("cannot add user: %v", err)
	}
	file, err := s.Files.New(context.Background(), user.ID, "mixed", "TXT", int64(len("abcdefg")))
	if err != nil {
		t.Fatalf("cannot create file: %v", err)
	}

	readerBlobs, err := s.Files.AddReaderBack(context.Background(), file.ID, strings.NewReader("abcd"))
	if err != nil {
		t.Fatalf("cannot add reader: %v", err)
	}
	if len(readerBlobs) != 1 {
		t.Fatalf("got %d reader blobs, expected 1", len(readerBlobs))
	}
	lastBlob, err := s.bs.AddData([]byte("efg"))
	if err != nil {
		t.Fatalf("cannot store standalone blob: %v", err)
	}
	if err := s.Files.AddBlobBack(context.Background(), file.ID, *lastBlob); err != nil {
		t.Fatalf("cannot append blob: %v", err)
	}
	if err := s.Files.Commit(context.Background(), file.ID); err != nil {
		t.Fatalf("cannot commit file: %v", err)
	}

	got, err := readAll(t, s, file.ID)
	if err != nil {
		t.Fatalf("cannot read file: %v", err)
	}
	if string(got) != "abcdefg" {
		t.Fatalf("got %q, expected %q", got, "abcdefg")
	}
}

func TestPositionedAddsReplaceAndPreserveOrder(t *testing.T) {
	previousMaxPartSize := blobs.BlobsMaxSize
	blobs.BlobsMaxSize = 2
	t.Cleanup(func() { blobs.BlobsMaxSize = previousMaxPartSize })

	s := getStorage(t)
	user, err := s.Users.New(context.Background(), "Positioned upload user")
	if err != nil {
		t.Fatalf("cannot add user: %v", err)
	}
	file, err := s.Files.New(context.Background(), user.ID, "positioned", "TXT", int64(len("aaBBccdd")))
	if err != nil {
		t.Fatalf("cannot create file: %v", err)
	}

	if _, err := s.Files.AddData(context.Background(), file.ID, 2, []byte("cc")); err != nil {
		t.Fatalf("cannot add data at position 2: %v", err)
	}
	if _, err := s.Files.AddReader(context.Background(), file.ID, 0, strings.NewReader("aa")); err != nil {
		t.Fatalf("cannot add reader at position 0: %v", err)
	}
	if _, err := s.Files.AddDataBack(context.Background(), file.ID, []byte("dd")); err != nil {
		t.Fatalf("cannot append data after a position gap: %v", err)
	}
	positionOne, err := s.bs.AddData([]byte("bb"))
	if err != nil {
		t.Fatalf("cannot store standalone blob: %v", err)
	}
	if err := s.Files.AddBlob(context.Background(), file.ID, 1, *positionOne); err != nil {
		t.Fatalf("cannot add blob at position 1: %v", err)
	}
	if _, err := s.Files.AddData(context.Background(), file.ID, 1, []byte("BB")); err != nil {
		t.Fatalf("cannot replace position 1: %v", err)
	}

	fileBlobs, err := s.Files.GetBlobs(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("cannot get file blobs: %v", err)
	}
	if len(fileBlobs) != 4 {
		t.Fatalf("got %d blobs, expected 4", len(fileBlobs))
	}
	if err := s.Files.Commit(context.Background(), file.ID); err != nil {
		t.Fatalf("cannot commit file: %v", err)
	}
	got, err := readAll(t, s, file.ID)
	if err != nil {
		t.Fatalf("cannot read file: %v", err)
	}
	if string(got) != "aaBBccdd" {
		t.Fatalf("got %q, expected %q", got, "aaBBccdd")
	}
}

func TestPositionedAddsValidatePositionAndSize(t *testing.T) {
	previousMaxPartSize := blobs.BlobsMaxSize
	blobs.BlobsMaxSize = 4
	t.Cleanup(func() { blobs.BlobsMaxSize = previousMaxPartSize })

	s := getStorage(t)
	user, err := s.Users.New(context.Background(), "Position validation user")
	if err != nil {
		t.Fatalf("cannot add user: %v", err)
	}
	file, err := s.Files.New(context.Background(), user.ID, "validation", "TXT", 0)
	if err != nil {
		t.Fatalf("cannot create file: %v", err)
	}
	blob, err := s.bs.AddData([]byte("data"))
	if err != nil {
		t.Fatalf("cannot store standalone blob: %v", err)
	}

	if _, err := s.Files.AddData(context.Background(), file.ID, -1, nil); !errors.Is(err, ErrInvalidPosition) {
		t.Fatalf("AddData returned %v, expected %v", err, ErrInvalidPosition)
	}
	if _, err := s.Files.AddReader(context.Background(), file.ID, -1, strings.NewReader("")); !errors.Is(err, ErrInvalidPosition) {
		t.Fatalf("AddReader returned %v, expected %v", err, ErrInvalidPosition)
	}
	if err := s.Files.AddBlob(context.Background(), file.ID, -1, *blob); !errors.Is(err, ErrInvalidPosition) {
		t.Fatalf("AddBlob returned %v, expected %v", err, ErrInvalidPosition)
	}
	if _, err := s.Files.AddData(context.Background(), file.ID, 0, []byte("12345")); !errors.Is(err, blobs.ErrBlobOverflow) {
		t.Fatalf("oversized AddData returned %v, expected %v", err, blobs.ErrBlobOverflow)
	}
	reader := strings.NewReader("12345")
	stored, err := s.Files.AddReader(context.Background(), file.ID, 0, reader)
	if err != nil {
		t.Fatalf("AddReader returned error for reader with remaining data: %v", err)
	}
	if stored.Size != blobs.BlobsMaxSize {
		t.Fatalf("AddReader stored %d bytes, expected %d", stored.Size, blobs.BlobsMaxSize)
	}
	remaining, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("cannot read remaining data: %v", err)
	}
	if string(remaining) != "5" {
		t.Fatalf("got remaining data %q, expected %q", remaining, "5")
	}

	fileBlobs, err := s.Files.GetBlobs(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("cannot get file blobs: %v", err)
	}
	if len(fileBlobs) != 1 || fileBlobs[0] != *stored {
		t.Fatalf("got blobs %+v, expected only %+v", fileBlobs, *stored)
	}
}

func TestPositionedAddsCanRunConcurrently(t *testing.T) {
	previousMaxPartSize := blobs.BlobsMaxSize
	blobs.BlobsMaxSize = 1
	t.Cleanup(func() { blobs.BlobsMaxSize = previousMaxPartSize })

	s := getStorage(t)
	user, err := s.Users.New(context.Background(), "Concurrent positioned upload user")
	if err != nil {
		t.Fatalf("cannot add user: %v", err)
	}

	const parts = 8
	file, err := s.Files.New(context.Background(), user.ID, "concurrent", "TXT", parts)
	if err != nil {
		t.Fatalf("cannot create file: %v", err)
	}

	errCh := make(chan error, parts)
	var wg sync.WaitGroup
	for position := 0; position < parts; position++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Files.AddData(context.Background(), file.ID, position, []byte{byte('a' + position)})
			errCh <- err
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent positioned add failed: %v", err)
		}
	}

	if err := s.Files.Commit(context.Background(), file.ID); err != nil {
		t.Fatalf("cannot commit concurrent upload: %v", err)
	}
	got, err := readAll(t, s, file.ID)
	if err != nil {
		t.Fatalf("cannot read concurrent upload: %v", err)
	}
	if string(got) != "abcdefgh" {
		t.Fatalf("got %q, expected %q", got, "abcdefgh")
	}
}

func TestValidateUploadParts(t *testing.T) {
	previousMaxPartSize := blobs.BlobsMaxSize
	blobs.BlobsMaxSize = 4
	t.Cleanup(func() { blobs.BlobsMaxSize = previousMaxPartSize })

	tests := []struct {
		name         string
		declaredSize int64
		parts        []UploadPart
		wantErr      bool
	}{
		{name: "empty upload", declaredSize: 0},
		{name: "complete upload", declaredSize: 10, parts: []UploadPart{{Position: 0, Size: 4}, {Position: 1, Size: 4}, {Position: 2, Size: 2}}},
		{name: "missing first position", declaredSize: 4, parts: []UploadPart{{Position: 1, Size: 4}}, wantErr: true},
		{name: "position gap", declaredSize: 6, parts: []UploadPart{{Position: 0, Size: 4}, {Position: 2, Size: 2}}, wantErr: true},
		{name: "short non-final part", declaredSize: 5, parts: []UploadPart{{Position: 0, Size: 3}, {Position: 1, Size: 2}}, wantErr: true},
		{name: "oversized final part", declaredSize: 5, parts: []UploadPart{{Position: 0, Size: 5}}, wantErr: true},
		{name: "size mismatch", declaredSize: 5, parts: []UploadPart{{Position: 0, Size: 4}}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUploadParts(tt.declaredSize, tt.parts)
			if tt.wantErr && !errors.Is(err, ErrEarlyCommit) {
				t.Fatalf("got error %v, expected %v", err, ErrEarlyCommit)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestRemoveBlobRemovesOnlyRequestedUploadPosition(t *testing.T) {
	s := getStorage(t)
	user, err := s.Users.New(context.Background(), "Part removal user")
	if err != nil {
		t.Fatalf("cannot add user: %v", err)
	}
	file, err := s.Files.New(context.Background(), user.ID, "removal", "TXT", 3)
	if err != nil {
		t.Fatalf("cannot create file: %v", err)
	}

	for position, value := range []byte("abc") {
		if _, err := s.Files.AddData(context.Background(), file.ID, position, []byte{value}); err != nil {
			t.Fatalf("cannot add part at position %d: %v", position, err)
		}
	}

	if err := s.Files.RemoveBlob(context.Background(), file.ID, 1); err != nil {
		t.Fatalf("cannot remove part: %v", err)
	}

	upload, err := s.Files.GetUpload(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("cannot get upload: %v", err)
	}
	expected := []UploadPart{{Position: 0, Size: 1}, {Position: 2, Size: 1}}
	if len(upload.Parts) != len(expected) {
		t.Fatalf("got %d parts, expected %d", len(upload.Parts), len(expected))
	}
	for i := range expected {
		if upload.Parts[i] != expected[i] {
			t.Errorf("got part %d %+v, expected %+v", i, upload.Parts[i], expected[i])
		}
	}
	if upload.ReceivedSize != 2 {
		t.Errorf("got received size %d, expected 2", upload.ReceivedSize)
	}

	storedBlobs, err := s.bs.ListBlobs()
	if err != nil {
		t.Fatalf("cannot list physical blobs: %v", err)
	}
	if len(storedBlobs) != 3 {
		t.Errorf("got %d physical blobs, expected all 3 to remain for garbage collection", len(storedBlobs))
	}

	if err := s.Files.RemoveBlob(context.Background(), file.ID, 1); !errors.Is(err, ErrBlobNotFound) {
		t.Fatalf("removing missing part returned %v, expected %v", err, ErrBlobNotFound)
	}
	if err := s.Files.RemoveBlob(context.Background(), file.ID, -1); !errors.Is(err, ErrInvalidPosition) {
		t.Fatalf("removing negative position returned %v, expected %v", err, ErrInvalidPosition)
	}
}

func TestCancelUploadRemovesOnlyUnfinishedMetadata(t *testing.T) {
	s := getStorage(t)
	user, err := s.Users.New(context.Background(), "Upload cancellation user")
	if err != nil {
		t.Fatalf("cannot add user: %v", err)
	}

	upload, err := s.Files.New(context.Background(), user.ID, "unfinished", "TXT", 4)
	if err != nil {
		t.Fatalf("cannot create upload: %v", err)
	}
	if _, err := s.Files.AddData(context.Background(), upload.ID, 0, []byte("part")); err != nil {
		t.Fatalf("cannot add upload part: %v", err)
	}

	if err := s.Files.CancelUpload(context.Background(), upload.ID); err != nil {
		t.Fatalf("cannot cancel upload: %v", err)
	}
	if _, err := s.Files.GetUpload(context.Background(), upload.ID); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("getting canceled upload returned %v, expected %v", err, ErrFileNotFound)
	}
	if _, err := s.Files.GetBlobs(context.Background(), upload.ID); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("getting canceled upload blobs returned %v, expected %v", err, ErrFileNotFound)
	}
	storedBlobs, err := s.bs.ListBlobs()
	if err != nil {
		t.Fatalf("cannot list physical blobs: %v", err)
	}
	if len(storedBlobs) != 1 {
		t.Errorf("got %d physical blobs, expected one pending garbage collection", len(storedBlobs))
	}

	ready, err := s.Files.UploadData(context.Background(), user.ID, "ready", "TXT", []byte("ready"))
	if err != nil {
		t.Fatalf("cannot create completed file: %v", err)
	}
	if err := s.Files.CancelUpload(context.Background(), ready.ID); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("canceling completed file returned %v, expected %v", err, ErrFileNotFound)
	}
	if _, err := s.Files.Get(context.Background(), ready.ID); err != nil {
		t.Fatalf("completed file changed after cancellation attempt: %v", err)
	}
}

func TestReadUsesCanceledContext(t *testing.T) {
	s := getStorage(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	dataCh, errCh := s.Files.Read(ctx, FileID("does-not-matter"))
	if chunk, ok := <-dataCh; ok {
		t.Fatalf("got data chunk %q for canceled read", chunk)
	}

	err, ok := <-errCh
	if !ok {
		t.Fatal("error channel closed without reporting cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got error %v, expected %v", err, context.Canceled)
	}
}

func TestReadNotFound(t *testing.T) {
	s := getStorage(t)

	_, err := readAll(t, s, GenerateFileID())

	if !errors.Is(err, ErrFileNotFound) {
		t.Fatalf(
			"got error %v, expected %v",
			err,
			ErrFileNotFound,
		)
	}
}

func TestGetBlobsNotFound(t *testing.T) {
	s := getStorage(t)

	_, err := s.Files.GetBlobs(context.Background(), GenerateFileID())
	if !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("got error %v, expected %v", err, ErrFileNotFound)
	}
}

func TestDeleteReturnsDatabaseErrorInsteadOfNotFound(t *testing.T) {
	s := getStorage(t)

	kotik, err := s.Users.New(context.Background(), "Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}

	file, err := s.Files.UploadData(context.Background(),
		kotik.ID,
		"Notebook",
		"TXT",
		[]byte("Hi man!"),
	)
	if err != nil {
		t.Fatalf("cannot upload Notebook: %v", err)
	}

	if err := s.db.Close(); err != nil {
		t.Fatalf("cannot close database: %v", err)
	}

	err = s.Files.Delete(context.Background(), file.ID)
	if err == nil {
		t.Fatal("expected database error")
	}
	if errors.Is(err, ErrFileNotFound) {
		t.Fatalf("got file not found error after database failure: %v", err)
	}
}

func TestDelete(t *testing.T) {
	s := getStorage(t)

	kotik, err := s.Users.New(context.Background(), "Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}

	file, err := s.Files.UploadData(context.Background(),
		kotik.ID,
		"Notebook",
		"TXT",
		[]byte("Hi man!"),
	)
	if err != nil {
		t.Fatalf("cannot upload Notebook: %v", err)
	}

	if err := s.Files.Delete(context.Background(), file.ID); err != nil {
		t.Fatalf("cannot delete Notebook: %v", err)
	}

	_, err = readAll(t, s, file.ID)

	if !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("Notebook was not deleted")
	}
}

func TestAddDataRejectsCommittedFile(t *testing.T) {
	s := getStorage(t)
	kotik, err := s.Users.New(context.Background(), "Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}
	file, err := s.Files.UploadData(context.Background(), kotik.ID, "Notebook", "TXT", []byte("data"))
	if err != nil {
		t.Fatalf("cannot upload file: %v", err)
	}
	_, err = s.Files.AddDataBack(context.Background(), file.ID, []byte("extra"))
	if !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("AddDataBack returned %v, expected %v", err, ErrFileNotFound)
	}
	_, err = s.Files.AddData(context.Background(), file.ID, 0, []byte("replacement"))
	if !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("positioned AddData returned %v, expected %v", err, ErrFileNotFound)
	}
	_, err = s.Files.AddData(context.Background(), GenerateFileID(), 0, []byte("missing"))
	if !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("AddData for missing file returned %v, expected %v", err, ErrFileNotFound)
	}
	loaded, err := s.Files.Get(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("cannot load committed file: %v", err)
	}
	fileBlobs, err := s.Files.GetBlobs(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("cannot get committed file blobs: %v", err)
	}
	if loaded.Size != int64(len("data")) || len(fileBlobs) != 1 {
		t.Fatalf("committed file changed after rejected add: %+v", loaded)
	}
}

func TestRename(t *testing.T) {
	s := getStorage(t)

	kotik, err := s.Users.New(context.Background(), "Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}

	file, err := s.Files.UploadData(context.Background(),
		kotik.ID,
		"Notebook",
		"TXT",
		[]byte("Hi man!"),
	)
	if err != nil {
		t.Fatalf("cannot upload Notebook: %v", err)
	}

	renamed, err := s.Files.Rename(context.Background(), file.ID, "Book")
	if err != nil {
		t.Fatalf("cannot rename Notebook: %v", err)
	}

	if renamed.Name != "Book" {
		t.Fatalf(
			"got name %q, expected %q",
			renamed.Name,
			"Book",
		)
	}

	file2, err := s.Files.Get(context.Background(), file.ID)

	if err != nil {
		t.Fatalf("GetFile returned error: %v", err)
	}
	if file2 == nil {
		t.Fatal("GetFile returned nil")
	}

	if file2.Name != "Book" {
		t.Fatalf(
			"got name %q, expected %q",
			file2.Name,
			"Book",
		)
	}
}

func testUpload(t *testing.T, data []byte) {
	t.Helper()
	s := getStorage(t)
	kotik, err := s.Users.New(context.Background(), "Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}
	file, err := s.Files.UploadData(context.Background(),
		kotik.ID,
		"Movie!",
		"TXT",
		data,
	)
	if err != nil {
		t.Fatalf("cannot upload file: %v", err)
	}

	got, err := readAll(t, s, file.ID)
	if err != nil {
		t.Fatalf("cannot read stored file: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("read unexpeced data")
	}
}

func TestUploadMovie80mb(t *testing.T) {
	filename := "testdata/movie.mov"
	data, err := os.ReadFile(filename)

	if err != nil {
		t.Fatalf("Cannot open movie.mov file: %v", err)
	}

	testUpload(t, data)
}

func TestUpload16mb(t *testing.T) {
	data := make([]byte, 16*1024*1024)
	for i := range data {
		data[i] = byte(i % (67 + 52))
	}
	testUpload(t, data)
}

func TestUpload16mbPlus1(t *testing.T) {
	data := make([]byte, 16*1024*1024+1)
	for i := range data {
		data[i] = byte(i % (67 + 52))
	}
	testUpload(t, data)
}
