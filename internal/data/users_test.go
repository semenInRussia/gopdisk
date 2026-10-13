package data

import (
	"context"
	"errors"
	"testing"
)

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

	files, err := s.Users.GetFiles(context.Background(), owner.ID)
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
