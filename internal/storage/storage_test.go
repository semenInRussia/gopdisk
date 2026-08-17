package storage

import (
	"bytes"
	"errors"
	"testing"
)

func getStorage(t *testing.T) *Storage {
	t.Helper()

	s, err := MakeStorage(t.TempDir())
	if err != nil {
		t.Fatalf("cannot create storage: %v", err)
	}

	return s
}

func readAll(t *testing.T, s *Storage, fileID FileID) ([]byte, error) {
	t.Helper()

	dataCh, errCh := s.Read(fileID)

	var data []byte

	for chunk := range dataCh {
		data = append(data, chunk...)
	}

	err, _ := <-errCh

	return data, err
}

func TestAddUser(t *testing.T) {
	s := getStorage(t)

	u, err := s.AddUser("Kotik")
	if err != nil {
		t.Fatalf("got error when adding user: %v", err)
	}

	if u.Name != "Kotik" {
		t.Fatalf("got %q, expected %q", u.Name, "Kotik")
	}

	u2, err := s.AddUser("Alex")
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

	got := s.GetUser(u.ID)

	if got == nil {
		t.Fatal("GetUser returned nil")
	}

	if got.ID != u.ID {
		t.Fatalf("got ID %q, expected %q", got.ID, u.ID)
	}
}

func TestUploadAndRead(t *testing.T) {
	s := getStorage(t)

	kotik, err := s.AddUser("Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}

	notebookText := []byte("Hi man!")

	file, err := s.Upload(
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

func TestDelete(t *testing.T) {
	s := getStorage(t)

	kotik, err := s.AddUser("Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}

	file, err := s.Upload(
		kotik.ID,
		"Notebook",
		"TXT",
		[]byte("Hi man!"),
	)
	if err != nil {
		t.Fatalf("cannot upload Notebook: %v", err)
	}

	if err := s.Delete(file.ID); err != nil {
		t.Fatalf("cannot delete Notebook: %v", err)
	}

	_, err = readAll(t, s, file.ID)

	if !errors.Is(err, ErrStorageFileNotFound) {
		t.Fatalf("Notebook was not deleted")
	}
}

func TestRename(t *testing.T) {
	s := getStorage(t)

	kotik, err := s.AddUser("Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}

	file, err := s.Upload(
		kotik.ID,
		"Notebook",
		"TXT",
		[]byte("Hi man!"),
	)
	if err != nil {
		t.Fatalf("cannot upload Notebook: %v", err)
	}

	renamed, err := s.Rename(file.ID, "Book")
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

	file2 := s.GetFile(file.ID)

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
