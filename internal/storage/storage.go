package storage

import (
	"errors"
	"gopherdisk/internal/blobs"

	"github.com/google/uuid"
)

type FileID string
type UserID string
type FileKind string

type User struct {
	ID   UserID
	Name string
}

type File struct {
	ID       FileID
	Name     string
	Filetype FileKind
	OwnerID  UserID
	Blob     blobs.Blob
}

type Storage struct {
	bs    blobs.BlobStorage
	files map[FileID]*File
	users map[UserID]*User
}

func MakeStorage(path string) (*Storage, error) {
	bs, err := blobs.MakeBlobStorage(path)
	if err != nil {
		return nil, err
	}

	s := Storage{
		bs:    *bs,
		files: make(map[FileID]*File),
		users: make(map[UserID]*User),
	}

	return &s, nil
}

func (s *Storage) AddUser(name string) (*User, error) {
	id := UserID(uuid.New().String())
	u := User{
		ID:   id,
		Name: name,
	}
	s.users[id] = &u
	return &u, nil
}

func (s *Storage) GetUser(userId UserID) *User {
	return s.users[userId]
}

func (s *Storage) GetFile(fileId FileID) *File {
	return s.files[fileId]
}

func (s *Storage) Upload(
	ownerId UserID,
	name string,
	fileType FileKind,
	data []byte,
) (*File, error) {
	blob, err := s.bs.AddBlob(data)

	if err != nil {
		return nil, err
	}

	f := File{
		ID:       FileID(uuid.New().String()),
		Name:     name,
		Filetype: fileType,
		OwnerID:  ownerId,
		Blob:     blob,
	}

	s.files[f.ID] = &f

	return &f, nil
}

var ErrStorageFileNotFound = errors.New("Could not find a file in storage")

func (s *Storage) Delete(fileId FileID) error {
	file := s.files[fileId]

	if file == nil {
		return ErrStorageFileNotFound
	}

	delete(s.files, fileId)

	return nil
}

func (s *Storage) Rename(fileId FileID, newName string) (*File, error) {
	return s.changeFile(fileId, func(file *File) error {
		file.Name = newName
		return nil
	})
}

func (s *Storage) UpdateData(fileId FileID, newData []byte) (*File, error) {
	return s.changeFile(fileId, func(file *File) error {
		blob, err := s.bs.AddBlob(newData)
		if err != nil {
			return err
		}
		file.Blob = blob
		return nil
	})
}

func (s *Storage) changeFile(fileId FileID, f func(*File) error) (*File, error) {
	file := s.files[fileId]
	if file == nil {
		return nil, ErrStorageFileNotFound
	}
	err := f(file)
	return file, err
}

func (s *Storage) Read(fileId FileID) ([]byte, error) {
	file := s.files[fileId]
	if file == nil {
		return nil, ErrStorageFileNotFound
	}
	return s.bs.ReadBlob(file.Blob)
}
