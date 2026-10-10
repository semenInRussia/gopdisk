package data

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"gopherdisk/internal/blobs"
	"gopherdisk/internal/database"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
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

	dataCh, errCh := s.Files.Read(context.Background(), fileID)

	var data []byte

	for chunk := range dataCh {
		data = append(data, chunk...)
	}

	err, _ := <-errCh

	return data, err
}

func TestAddUser(t *testing.T) {
	s := getStorage(t)

	u, err := s.Users.New(context.Background(), "Kotik")
	if err != nil {
		t.Fatalf("got error when adding user: %v", err)
	}

	if u.Name != "Kotik" {
		t.Fatalf("got %q, expected %q", u.Name, "Kotik")
	}

	u2, err := s.Users.New(context.Background(), "Alex")
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

	got, err := s.Users.Get(context.Background(), u.ID)

	if got == nil {
		t.Fatal("GetUser returned nil")
	}
	if err != nil {
		t.Fatalf("GetUser returned error: %v", err)
	}

	if got.ID != u.ID {
		t.Fatalf("got ID %q, expected %q", got.ID, u.ID)
	}
}

func TestAddUserUsesCanceledContext(t *testing.T) {
	s := getStorage(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.Users.New(ctx, "Kotik")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got error %v, expected %v", err, context.Canceled)
	}
}

func TestGetUserReturnsNotFound(t *testing.T) {
	s := getStorage(t)

	got, err := s.Users.Get(context.Background(), GenerateUserID())
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("got error %v, expected %v", err, ErrUserNotFound)
	}
	if got != nil {
		t.Fatalf("got user %+v, expected nil", got)
	}
}

func TestInsertUserPersistsConstructedUser(t *testing.T) {
	s := getStorage(t)
	user := User{ID: GenerateUserID(), Name: "Constructed"}
	if err := s.Users.Insert(context.Background(), user); err != nil {
		t.Fatalf("cannot insert user: %v", err)
	}
	got, err := s.Users.Get(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("cannot get inserted user: %v", err)
	}
	if got == nil || *got != user {
		t.Fatalf("got %+v, expected %+v", got, user)
	}
}

func TestInsertFilePersistsConstructedFile(t *testing.T) {
	s := getStorage(t)
	kotik, err := s.Users.New(context.Background(), "Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}
	file := File{
		ID: GenerateFileID(), Name: "Empty", Filetype: "TXT", OwnerID: kotik.ID,
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
		ID: GenerateFileID(), Name: "Invalid", Filetype: "TXT", OwnerID: kotik.ID,
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

func TestDeleteUser(t *testing.T) {
	s := getStorage(t)

	user, err := s.Users.New(context.Background(), "Delete Me")
	if err != nil {
		t.Fatalf("cannot add user: %v", err)
	}

	if err := s.Users.Delete(context.Background(), user.ID); err != nil {
		t.Fatalf("cannot delete user: %v", err)
	}

	got, err := s.Users.Get(context.Background(), user.ID)
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("got error %v, expected %v", err, ErrUserNotFound)
	}
	if got != nil {
		t.Fatalf("got deleted user %+v, expected nil", got)
	}
}

func TestDeleteUserReturnsNotFound(t *testing.T) {
	s := getStorage(t)

	err := s.Users.Delete(context.Background(), GenerateUserID())
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("got error %v, expected %v", err, ErrUserNotFound)
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
	for i, part := range [][]byte{[]byte("hel"), []byte("lo-world")} {
		if _, err := s.Files.AddData(context.Background(), file.ID, part); err != nil {
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
	for i, expectedSize := range []int{3, 4, 4} {
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
	if _, err := s.Files.AddData(context.Background(), file.ID, []byte("four")); err != nil {
		t.Fatalf("cannot add data: %v", err)
	}
	if err := s.Files.Commit(context.Background(), file.ID); !errors.Is(err, ErrEarlyCommit) {
		t.Fatalf("got error %v, expected %v", err, ErrEarlyCommit)
	}
	if _, err := s.Files.AddData(context.Background(), file.ID, []byte("!")); err != nil {
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
		{name: "small", data: "abc"},
		{name: "exact", data: "abcd"},
		{name: "plus-one", data: "abcde"},
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

	readerBlobs, err := s.Files.AddReader(context.Background(), file.ID, strings.NewReader("abcde"))
	if err != nil {
		t.Fatalf("cannot add reader: %v", err)
	}
	if len(readerBlobs) != 2 {
		t.Fatalf("got %d reader blobs, expected 2", len(readerBlobs))
	}
	lastBlob, err := s.bs.AddOneChunk([]byte("fg"))
	if err != nil {
		t.Fatalf("cannot store standalone blob: %v", err)
	}
	if err := s.Files.AddBlob(context.Background(), file.ID, *lastBlob); err != nil {
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

func TestListFilesReturnsReadyFilesForOwner(t *testing.T) {
	s := getStorage(t)
	owner, err := s.Users.New(context.Background(), "Owner")
	if err != nil {
		t.Fatalf("cannot add owner: %v", err)
	}
	other, err := s.Users.New(context.Background(), "Other")
	if err != nil {
		t.Fatalf("cannot add other user: %v", err)
	}

	alpha, err := s.Files.UploadData(context.Background(), owner.ID, "alpha", "TXT", []byte("alpha"))
	if err != nil {
		t.Fatalf("cannot upload alpha: %v", err)
	}
	charlie, err := s.Files.UploadData(context.Background(), owner.ID, "charlie", "TXT", []byte("charlie"))
	if err != nil {
		t.Fatalf("cannot upload charlie: %v", err)
	}
	if _, err := s.Files.New(context.Background(), owner.ID, "bravo", "TXT", 5); err != nil {
		t.Fatalf("cannot create unfinished upload: %v", err)
	}
	if _, err := s.Files.UploadData(context.Background(), other.ID, "delta", "TXT", []byte("delta")); err != nil {
		t.Fatalf("cannot upload other user file: %v", err)
	}

	files, err := s.Files.List(context.Background(), owner.ID)
	if err != nil {
		t.Fatalf("cannot list files: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d files, expected 2", len(files))
	}
	if files[0].ID != alpha.ID || files[1].ID != charlie.ID {
		t.Fatalf("got files %v, expected alpha then charlie", files)
	}
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
	_, err = s.Files.AddData(context.Background(), file.ID, []byte("extra"))
	if !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("got error %v, expected %v", err, ErrFileNotFound)
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
