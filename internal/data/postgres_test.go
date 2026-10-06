package data

import (
	"context"
	"os"
	"testing"

	"gopherdisk/internal/database"
)

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

	user, err := s.Users.Add(context.Background(), "Postgres User")
	if err != nil {
		t.Fatalf("cannot add user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id = $1`, user.ID)
	})

	data := []byte("postgres backed file")
	file, err := s.Files.Upload(context.Background(), user.ID, "db-file", "TXT", data)
	if err != nil {
		t.Fatalf("cannot upload file: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM files WHERE id = $1`, file.ID)
		for _, blob := range file.Blobs {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM blobs WHERE hash = $1`, blob.Hash)
		}
	})

	loadedStorage, err := MakeStorage(dir, db)
	if err != nil {
		t.Fatalf("cannot create second postgres storage: %v", err)
	}

	loadedFile, err := loadedStorage.Files.GetFile(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("got error from GetFile %v", err)
	}
	if loadedFile == nil {
		t.Fatal("GetFile returned nil")
	}
	if loadedFile.ID != file.ID {
		t.Fatalf("got file ID %q, expected %q", loadedFile.ID, file.ID)
	}
	if len(loadedFile.Blobs) != len(file.Blobs) {
		t.Fatalf("got %d blobs, expected %d", len(loadedFile.Blobs), len(file.Blobs))
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
	afterRename, err := afterRenameStorage.Files.GetFile(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("cannot GetFile after rename: %v", err)
	}
	if afterRename == nil {
		t.Fatal("GetFile after rename returned nil")
	}
	if afterRename.Name != "renamed-db-file" {
		t.Fatalf("got persisted name %q, expected %q", afterRename.Name, "renamed-db-file")
	}

	updatedData := []byte("updated postgres backed file")
	updated, err := afterRenameStorage.Files.UpdateData(context.Background(), file.ID, updatedData)
	if err != nil {
		t.Fatalf("cannot update file data: %v", err)
	}
	for _, blob := range updated.Blobs {
		t.Cleanup(func() {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM blobs WHERE hash = $1`, blob.Hash)
		})
	}

	afterUpdateStorage, err := MakeStorage(dir, db)
	if err != nil {
		t.Fatalf("cannot create storage after update: %v", err)
	}
	got, err = readAll(t, afterUpdateStorage, file.ID)
	if err != nil {
		t.Fatalf("cannot read updated file: %v", err)
	}
	if string(got) != string(updatedData) {
		t.Fatalf("got updated data %q, expected %q", got, updatedData)
	}

	if err := afterUpdateStorage.Files.Delete(context.Background(), file.ID); err != nil {
		t.Fatalf("cannot delete file: %v", err)
	}

	afterDeleteStorage, err := MakeStorage(dir, db)
	if err != nil {
		t.Fatalf("cannot create storage after delete: %v", err)
	}
	gotFile, err := afterDeleteStorage.Files.GetFile(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("GetFile after delete returned err: %v", err)
	}
	if gotFile != nil {
		t.Fatalf("GetFile after delete returned file %q", gotFile.ID)
	}
}
