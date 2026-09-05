package storage

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"gopherdisk/internal/blobs"
	"gopherdisk/internal/database"
	"os"
	"path/filepath"
	"testing"
)

const (
	databaseTestURLEnv                  = "GOPHERDISK_DATABASE_TEST_URL"
	garbageCollectionDatabaseTestURLEnv = "GOPHERDISK_GARBAGE_COLLECTION_TEST_URL"
)

func getStorage(t *testing.T) *Storage {
	t.Helper()

	db := getDatabase(t)
	dir := t.TempDir()
	s, err := MakeStorage(dir, db)

	if err != nil {
		t.Fatalf("cannot create storage: %v", err)
	}

	return s
}

func getDatabase(t *testing.T) *sql.DB {
	t.Helper()

	databaseURL := os.Getenv(databaseTestURLEnv)
	if databaseURL == "" {
		t.Skipf("set %s to run storage tests", databaseTestURLEnv)
	}

	db, err := database.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("cannot open database: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func readAll(t *testing.T, s *Storage, fileID FileID) ([]byte, error) {
	t.Helper()

	dataCh, errCh := s.Read(context.Background(), fileID)

	var data []byte

	for chunk := range dataCh {
		data = append(data, chunk...)
	}

	err, _ := <-errCh

	return data, err
}

func TestAddUser(t *testing.T) {
	s := getStorage(t)

	u, err := s.AddUser(context.Background(), "Kotik")
	if err != nil {
		t.Fatalf("got error when adding user: %v", err)
	}

	if u.Name != "Kotik" {
		t.Fatalf("got %q, expected %q", u.Name, "Kotik")
	}

	u2, err := s.AddUser(context.Background(), "Alex")
	if err != nil {
		t.Fatalf("got error when adding second user: %v", err)
	}

	if u2.Name != "Alex" {
		t.Fatalf("got %q, expected %q", u2.Name, "Alex")
	}

	if u.ID == u2.ID {
		t.Fatalf(
			"two different users have the same ID: %q",
			u.ID,
		)
	}

	got := s.GetUser(context.Background(), u.ID)

	if got == nil {
		t.Fatal("GetUser returned nil")
	}

	if got.ID != u.ID {
		t.Fatalf("got ID %q, expected %q", got.ID, u.ID)
	}
}

func TestAddUserUsesCanceledContext(t *testing.T) {
	s := getStorage(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.AddUser(ctx, "Kotik")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got error %v, expected %v", err, context.Canceled)
	}
}

func TestUploadAndRead(t *testing.T) {
	s := getStorage(t)

	kotik, err := s.AddUser(context.Background(), "Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}

	notebookText := []byte("Hi man!")

	file, err := s.Upload(context.Background(),
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

func TestPreUploadPartsFinishAndRead(t *testing.T) {
	previousMaxPartSize := blobs.BlobsMaxSize
	blobs.BlobsMaxSize = 4
	t.Cleanup(func() {
		blobs.BlobsMaxSize = previousMaxPartSize
	})

	s := getStorage(t)

	kotik, err := s.AddUser(context.Background(), "Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}

	file, err := s.PreUpload(context.Background(), kotik.ID, "Notebook", "TXT", len("hello-world"))
	if err != nil {
		t.Fatalf("cannot create pre-upload file: %v", err)
	}

	parts := [][]byte{
		[]byte("hell"),
		[]byte("o-wo"),
		[]byte("rld"),
	}
	for i, part := range parts {
		if _, err := s.UploadPart(context.Background(), file.ID, i, part); err != nil {
			t.Fatalf("cannot upload part %d: %v", i, err)
		}
	}

	if err := s.FinishUpload(context.Background(), file.ID); err != nil {
		t.Fatalf("cannot finish upload: %v", err)
	}

	got, err := readAll(t, s, file.ID)
	if err != nil {
		t.Fatalf("cannot read finished file: %v", err)
	}

	if !bytes.Equal(got, []byte("hello-world")) {
		t.Fatalf("got %q, expected %q", got, "hello-world")
	}
}

func TestReadNotFound(t *testing.T) {
	s := getStorage(t)

	_, err := readAll(t, s, FileID("does-not-exist"))

	if !errors.Is(err, ErrStorageFileNotFound) {
		t.Fatalf(
			"got error %v, expected %v",
			err,
			ErrStorageFileNotFound,
		)
	}
}

func TestDeleteReturnsDatabaseErrorInsteadOfNotFound(t *testing.T) {
	s := getStorage(t)

	kotik, err := s.AddUser(context.Background(), "Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}

	file, err := s.Upload(context.Background(),
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

	err = s.Delete(context.Background(), file.ID)
	if err == nil {
		t.Fatal("expected database error")
	}
	if errors.Is(err, ErrStorageFileNotFound) {
		t.Fatalf("got file not found error after database failure: %v", err)
	}
}

func TestDelete(t *testing.T) {
	s := getStorage(t)

	kotik, err := s.AddUser(context.Background(), "Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}

	file, err := s.Upload(context.Background(),
		kotik.ID,
		"Notebook",
		"TXT",
		[]byte("Hi man!"),
	)
	if err != nil {
		t.Fatalf("cannot upload Notebook: %v", err)
	}

	if err := s.Delete(context.Background(), file.ID); err != nil {
		t.Fatalf("cannot delete Notebook: %v", err)
	}

	_, err = readAll(t, s, file.ID)

	if !errors.Is(err, ErrStorageFileNotFound) {
		t.Fatalf("Notebook was not deleted")
	}
}

func TestUploadPartRejectsPositionOutsideDeclaredFileSize(t *testing.T) {
	s := getStorage(t)

	kotik, err := s.AddUser(context.Background(), "Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}

	file, err := s.PreUpload(context.Background(), kotik.ID, "Notebook", "TXT", 1)
	if err != nil {
		t.Fatalf("cannot create pre-upload file: %v", err)
	}

	_, err = s.UploadPart(context.Background(), file.ID, 1, []byte("extra"))
	if err == nil {
		t.Fatal("expected error for upload part outside declared file size")
	}

	loaded := s.loadFile(context.Background(), file.ID, FileStatusUploading)
	if loaded == nil {
		t.Fatal("cannot load uploading file")
	}
	if len(loaded.Blobs) != 0 {
		t.Fatalf("got %d attached blobs, expected 0", len(loaded.Blobs))
	}
}

func TestCollectGarbageBlobsDeletesUnreferencedBlob(t *testing.T) {
	s := getGarbageCollectionStorage(t)

	blob, err := s.bs.AddOneChunk([]byte("unused blob"))
	if err != nil {
		t.Fatalf("cannot write blob file: %v", err)
	}

	_, err = s.db.ExecContext(
		context.Background(),
		`INSERT INTO blobs (hash, size) VALUES ($1, $2)`,
		blob.Hash,
		blob.Size,
	)
	if err != nil {
		t.Fatalf("cannot insert blob row: %v", err)
	}

	deleted, err := s.CollectGarbageBlobs(context.Background())
	if err != nil {
		t.Fatalf("cannot collect garbage blobs: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("got %d deleted blobs, expected 1", deleted)
	}
	if blobRowExists(t, s.db, blob.Hash) {
		t.Fatalf("blob row %q still exists", blob.Hash)
	}
	if blobFileExists(t, s, *blob) {
		t.Fatalf("blob file %q still exists", blob.Hash)
	}
}

func TestCollectGarbageBlobsKeepsReferencedBlob(t *testing.T) {
	s := getGarbageCollectionStorage(t)

	kotik, err := s.AddUser(context.Background(), "Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}

	file, err := s.Upload(context.Background(), kotik.ID, "Notebook", "TXT", []byte("Hi man!"))
	if err != nil {
		t.Fatalf("cannot upload Notebook: %v", err)
	}

	deleted, err := s.CollectGarbageBlobs(context.Background())
	if err != nil {
		t.Fatalf("cannot collect garbage blobs: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("got %d deleted blobs, expected 0", deleted)
	}

	for _, blob := range file.Blobs {
		if !blobRowExists(t, s.db, blob.Hash) {
			t.Fatalf("referenced blob row %q was deleted", blob.Hash)
		}
		if !blobFileExists(t, s, blob) {
			t.Fatalf("referenced blob file %q was deleted", blob.Hash)
		}
	}

	got, err := readAll(t, s, file.ID)
	if err != nil {
		t.Fatalf("cannot read file after garbage collection: %v", err)
	}
	if !bytes.Equal(got, []byte("Hi man!")) {
		t.Fatalf("got %q, expected %q", got, "Hi man!")
	}
}

func TestCollectGarbageBlobsAfterDelete(t *testing.T) {
	s := getGarbageCollectionStorage(t)

	kotik, err := s.AddUser(context.Background(), "Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}

	file, err := s.Upload(context.Background(), kotik.ID, "Notebook", "TXT", []byte("delete garbage unique blob"))
	if err != nil {
		t.Fatalf("cannot upload Notebook: %v", err)
	}

	uniqueBlobs := uniqueFileBlobs(file)
	if err := s.Delete(context.Background(), file.ID); err != nil {
		t.Fatalf("cannot delete file: %v", err)
	}

	deleted, err := s.CollectGarbageBlobs(context.Background())
	if err != nil {
		t.Fatalf("cannot collect garbage blobs: %v", err)
	}
	if deleted != len(uniqueBlobs) {
		t.Fatalf("got %d deleted blobs, expected %d", deleted, len(uniqueBlobs))
	}

	for _, blob := range uniqueBlobs {
		if blobRowExists(t, s.db, blob.Hash) {
			t.Fatalf("deleted file blob row %q still exists", blob.Hash)
		}
		if blobFileExists(t, s, blob) {
			t.Fatalf("deleted file blob %q still exists on disk", blob.Hash)
		}
	}
}

func TestCollectGarbageBlobsReturnsFilesystemError(t *testing.T) {
	s := getGarbageCollectionStorage(t)

	missingBlob := blobs.Blob{Hash: "missing-physical-blob", Size: 4}
	_, err := s.db.ExecContext(
		context.Background(),
		`INSERT INTO blobs (hash, size) VALUES ($1, $2)`,
		missingBlob.Hash,
		missingBlob.Size,
	)
	if err != nil {
		t.Fatalf("cannot insert blob row: %v", err)
	}

	deleted, err := s.CollectGarbageBlobs(context.Background())
	if err == nil {
		t.Fatal("expected filesystem error")
	}
	if deleted != 0 {
		t.Fatalf("got %d deleted blobs before error, expected 0", deleted)
	}
	if blobRowExists(t, s.db, missingBlob.Hash) {
		t.Fatalf("blob row %q still exists", missingBlob.Hash)
	}
}

func getGarbageCollectionStorage(t *testing.T) *Storage {
	t.Helper()

	databaseURL := os.Getenv(garbageCollectionDatabaseTestURLEnv)
	if databaseURL == "" {
		t.Skipf("set %s to run garbage collection storage tests", garbageCollectionDatabaseTestURLEnv)
	}

	db, err := database.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("cannot open database: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
	})

	s, err := MakeStorage(t.TempDir(), db)
	if err != nil {
		t.Fatalf("cannot create storage: %v", err)
	}

	return s
}

func blobRowExists(t *testing.T, db *sql.DB, hash string) bool {
	t.Helper()

	row := db.QueryRowContext(context.Background(), `SELECT EXISTS (SELECT 1 FROM blobs WHERE hash = $1)`, hash)
	var exists bool
	if err := row.Scan(&exists); err != nil {
		t.Fatalf("cannot check blob row: %v", err)
	}
	return exists
}

func blobFileExists(t *testing.T, s *Storage, blob blobs.Blob) bool {
	t.Helper()

	_, err := os.Stat(filepath.Join(s.bs.Path, blob.Hash))
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	t.Fatalf("cannot stat blob file: %v", err)
	return false
}

func uniqueFileBlobs(file *File) []blobs.Blob {
	seen := make(map[string]bool)
	var unique []blobs.Blob

	for _, blob := range file.Blobs {
		if seen[blob.Hash] {
			continue
		}
		seen[blob.Hash] = true
		unique = append(unique, blob)
	}

	return unique
}

func TestRename(t *testing.T) {
	s := getStorage(t)

	kotik, err := s.AddUser(context.Background(), "Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}

	file, err := s.Upload(context.Background(),
		kotik.ID,
		"Notebook",
		"TXT",
		[]byte("Hi man!"),
	)
	if err != nil {
		t.Fatalf("cannot upload Notebook: %v", err)
	}

	renamed, err := s.Rename(context.Background(), file.ID, "Book")
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

	file2 := s.GetFile(context.Background(), file.ID)

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
	kotik, err := s.AddUser(context.Background(), "Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}
	file, err := s.Upload(context.Background(),
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
