package storage

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

func getStorage() (*Storage, error) {
	return MakeStorage("testStorage")
}

func dropStorage(s *Storage) error {
	return os.RemoveAll(s.bs.Path)
}

func TestAddUser(t *testing.T) {
	s, err := getStorage()
	if err != nil {
		t.Fatalf("got error get storage: %v", err)
	}
	defer dropStorage(s)

	u, err := s.AddUser("Kotik")

	if err != nil {
		t.Fatalf("got error when add user: %v", err)
	}

	if u.Name != "Kotik" {
		t.Fatalf("got '%v', but expected 'Kotik'", u.Name)
	}

	u2, err := s.AddUser("Alex")

	if err != nil {
		t.Fatalf("got error when add 2nd user: %v", err)
	}

	if u2.Name != "Alex" {
		t.Fatalf("got '%v', but expected 'Alex'", u2.Name)
	}

	if u.ID == u2.ID {
		t.Fatalf("two different users have the same IDs: '%v' and '%v'", u.ID, u2.ID)
	}

	var v = s.GetUser(u.ID)
	if v == nil {
		t.Fatal("`GetUser` returns nil")
	}
	if u.ID != v.ID {
		t.Fatalf("got '%v', expected '%v'", v.ID, u.ID)
	}
}

func TestUploadAndRead(t *testing.T) {
	s, err := getStorage()
	if err != nil {
		t.Fatalf("got error get storage: %v", err)
	}
	defer dropStorage(s)

	kotik, err := s.AddUser("Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}

	notebookText := []byte("Hi man!")
	file, err := s.Upload(kotik.ID, "Notebook", "TXT", notebookText)
	if err != nil {
		t.Fatalf("cannot upload a Notebook: %v", err)
	}

	data, err := s.Read(file.ID)
	if err != nil {
		t.Fatalf("cannot read a Notebook: %v", err)
	}

	if !bytes.Equal(data, notebookText) {
		t.Fatalf("got wrong notebook text '%v', but expected: %v", data, notebookText)
	}
}

func TestDelete(t *testing.T) {
	s, err := getStorage()
	if err != nil {
		t.Fatalf("got error get storage: %v", err)
	}
	defer dropStorage(s)

	kotik, err := s.AddUser("Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}

	notebookText := []byte("Hi man!")
	file, err := s.Upload(kotik.ID, "Notebook", "TXT", notebookText)
	if err != nil {
		t.Fatalf("cannot upload a Notebook: %v", err)
	}

	err = s.Delete(file.ID)
	if err != nil {
		t.Fatalf("cannot delete a Notebook: %v", err)
	}

	_, err = s.Read(file.ID)
	if !errors.Is(err, ErrStorageFileNotFound) {
		t.Fatalf("Notebook was not deleted")
	}
}

func TestRename(t *testing.T) {
	s, err := getStorage()
	if err != nil {
		t.Fatalf("got error when get storage: %v", err)
	}
	defer dropStorage(s)

	kotik, err := s.AddUser("Kotik")
	if err != nil {
		t.Fatalf("cannot add user Kotik: %v", err)
	}

	notebookText := []byte("Hi man!")
	file, err := s.Upload(kotik.ID, "Notebook", "TXT", notebookText)
	if err != nil {
		t.Fatalf("cannot upload a Notebook: %v", err)
	}

	_, err = s.Rename(file.ID, "Book")
	if err != nil {
		t.Fatalf("cannot rename a Notebook: %v", err)
	}

	file2 := s.GetFile(file.ID)
	if file2 == nil {
		t.Fatalf("cannot found a Notebook(Book)")
	}
	if file2.Name != "Book" {
		t.Fatalf("cannot found a Notebook(Book)")
	}
}
