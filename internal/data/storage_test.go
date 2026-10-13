package data

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path"
	"path/filepath"
	"testing"

	"gopherdisk/internal/blobs"
	"gopherdisk/internal/database"
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

func TestFileBelongsToUser(t *testing.T) {
	s := getStorage(t)
	owner, err := s.Users.New(context.Background(), "Owner")
	if err != nil {
		t.Fatalf("cannot add owner: %v", err)
	}
	other, err := s.Users.New(context.Background(), "Other")
	if err != nil {
		t.Fatalf("cannot add other user: %v", err)
	}

	file, err := s.Files.UploadData(context.Background(), owner.ID, "Notebook", "TXT", []byte("data"))
	if err != nil {
		t.Fatalf("cannot upload file: %v", err)
	}

	belongs, err := s.IsBelongsToUser(context.Background(), file.ID, owner.ID)
	if err != nil {
		t.Fatalf("cannot check owner relationship: %v", err)
	}
	if !belongs {
		t.Fatal("expected file to belong to owner")
	}

	belongs, err = s.IsBelongsToUser(context.Background(), file.ID, other.ID)
	if err != nil {
		t.Fatalf("cannot check other user relationship: %v", err)
	}
	if belongs {
		t.Fatal("expected file not to belong to other user")
	}

	belongs, err = s.IsBelongsToUser(context.Background(), FileID("00000000-0000-0000-0000-000000000000"), owner.ID)
	if err != nil {
		t.Fatalf("cannot check missing file relationship: %v", err)
	}
	if belongs {
		t.Fatal("expected missing file not to belong to owner")
	}
}

func TestCollectGarbageBlobsDeletesUnreferencedBlob(t *testing.T) {
	s := getGarbageCollectionStorage(t)

	blob, err := s.bs.AddData([]byte("unused blob"))
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

func TestCollectGarbageDeletesBlobsOutsideDb(t *testing.T) {
	s := getGarbageCollectionStorage(t)

	hash := "hash"
	content := []byte("sample text")

	err := os.WriteFile(path.Join(s.bs.Path, hash), content, 0644)
	if err != nil {
		t.Fatalf("cannot create a blob manually")
	}

	deleted, err := s.CollectGarbageBlobs(context.Background())
	if err != nil {
		t.Fatalf("cannot collect garbage blobs: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("got %d deleted blobs, expected 1", deleted)
	}

	blob := blobs.Blob{Hash: hash, Size: int64(len(content))}

	if blobFileExists(t, s, blob) {
		t.Fatalf("orphaned blob file %q still exists", blob.Hash)
	}
}

func TestCollectGarbageBlobsKeepsReferencedBlob(t *testing.T) {
	s := getGarbageCollectionStorage(t)

	kotik, err := s.Users.New(context.Background(), "Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}

	file, err := s.Files.UploadData(context.Background(), kotik.ID, "Notebook", "TXT", []byte("Hi man!"))
	if err != nil {
		t.Fatalf("cannot upload Notebook: %v", err)
	}
	fileBlobs, err := s.Files.GetBlobs(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("cannot get file blobs: %v", err)
	}

	deleted, err := s.CollectGarbageBlobs(context.Background())
	if err != nil {
		t.Fatalf("cannot collect garbage blobs: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("got %d deleted blobs, expected 0", deleted)
	}

	for _, blob := range fileBlobs {
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

	kotik, err := s.Users.New(context.Background(), "Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}

	file, err := s.Files.UploadData(context.Background(), kotik.ID, "Notebook", "TXT", []byte("delete garbage unique blob"))
	if err != nil {
		t.Fatalf("cannot upload Notebook: %v", err)
	}
	fileBlobs, err := s.Files.GetBlobs(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("cannot get file blobs: %v", err)
	}
	uniqueBlobs := uniqueFileBlobs(fileBlobs)
	if err := s.Files.Delete(context.Background(), file.ID); err != nil {
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

func uniqueFileBlobs(fileBlobs []blobs.Blob) []blobs.Blob {
	seen := make(map[string]bool)
	var unique []blobs.Blob

	for _, blob := range fileBlobs {
		if seen[blob.Hash] {
			continue
		}
		seen[blob.Hash] = true
		unique = append(unique, blob)
	}

	return unique
}

func TestPostgresStoragePersistsAndLoadsFile(t *testing.T) {
	databaseURL := os.Getenv(databaseTestURLEnv)
	if databaseURL == "" {
		t.Skipf("set %s to run PostgreSQL storage integration tests", databaseTestURLEnv)
	}

	db, err := database.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("cannot open database: %v", err)
	}
	defer db.Close()

	dir := t.TempDir()
	s, err := MakeStorage(dir, db)
	if err != nil {
		t.Fatalf("cannot create postgres storage: %v", err)
	}

	user, err := s.Users.New(context.Background(), "Postgres User")
	if err != nil {
		t.Fatalf("cannot add user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id = $1`, user.ID)
	})

	data := []byte("postgres backed file")
	file, err := s.Files.UploadData(context.Background(), user.ID, "db-file", "TXT", data)
	if err != nil {
		t.Fatalf("cannot upload file: %v", err)
	}
	fileBlobs, err := s.Files.GetBlobs(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("cannot get uploaded file blobs: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM files WHERE id = $1`, file.ID)
		for _, blob := range fileBlobs {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM blobs WHERE hash = $1`, blob.Hash)
		}
	})

	loadedStorage, err := MakeStorage(dir, db)
	if err != nil {
		t.Fatalf("cannot create second postgres storage: %v", err)
	}

	loadedFile, err := loadedStorage.Files.Get(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("got error from GetFile %v", err)
	}
	if loadedFile == nil {
		t.Fatal("GetFile returned nil")
	}
	if loadedFile.ID != file.ID {
		t.Fatalf("got file ID %q, expected %q", loadedFile.ID, file.ID)
	}
	loadedBlobs, err := loadedStorage.Files.GetBlobs(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("cannot get loaded file blobs: %v", err)
	}
	if len(loadedBlobs) != len(fileBlobs) {
		t.Fatalf("got %d blobs, expected %d", len(loadedBlobs), len(fileBlobs))
	}

	got, err := readAll(t, loadedStorage, file.ID)
	if err != nil {
		t.Fatalf("cannot read loaded file: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("got data %q, expected %q", got, data)
	}

	renamed, err := loadedStorage.Files.Rename(context.Background(), file.ID, "renamed-db-file")
	if err != nil {
		t.Fatalf("cannot rename file: %v", err)
	}
	if renamed.Name != "renamed-db-file" {
		t.Fatalf("got name %q, expected %q", renamed.Name, "renamed-db-file")
	}

	afterRenameStorage, err := MakeStorage(dir, db)
	if err != nil {
		t.Fatalf("cannot create storage after rename: %v", err)
	}
	afterRename, err := afterRenameStorage.Files.Get(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("cannot GetFile after rename: %v", err)
	}
	if afterRename == nil {
		t.Fatal("GetFile after rename returned nil")
	}
	if afterRename.Name != "renamed-db-file" {
		t.Fatalf("got persisted name %q, expected %q", afterRename.Name, "renamed-db-file")
	}

	if err := afterRenameStorage.Files.Delete(context.Background(), file.ID); err != nil {
		t.Fatalf("cannot delete file: %v", err)
	}

	afterDeleteStorage, err := MakeStorage(dir, db)
	if err != nil {
		t.Fatalf("cannot create storage after delete: %v", err)
	}
	gotFile, err := afterDeleteStorage.Files.Get(context.Background(), file.ID)
	if !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("got error %v, expected %v", err, ErrFileNotFound)
	}
	if gotFile != nil {
		t.Fatalf("GetFile after delete returned file %q", gotFile.ID)
	}
}
