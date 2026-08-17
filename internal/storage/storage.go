package storage

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"

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
	Size     int
	Hash     string
}

type Storage struct {
	Path  string
	files map[FileID]*File
	users map[UserID]*User
}

func MakeStorage(path string) (*Storage, error) {
	s := Storage{
		Path:  path,
		files: make(map[FileID]*File),
		users: make(map[UserID]*User),
	}
	err := os.Mkdir(path, 0755)
	if os.IsExist(err) || err == nil {
		return &s, nil
	}
	return nil, err
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
	hash, err := s.writeBlob(data)

	if err != nil {
		return nil, err
	}

	f := File{
		ID:       FileID(uuid.New().String()),
		Name:     name,
		Filetype: fileType,
		OwnerID:  ownerId,
		Size:     len(data),
		Hash:     hash,
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
		newHash, err := s.writeBlob(newData)
		if err != nil {
			return err
		}
		file.Hash = newHash
		file.Size = len(newData)
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
	return s.readBlob(file.Hash)
}

// blobs
//
// every blob is a real file in file system that responds on struct File that visible for user

// write data to correct file on file system, return hash of given data
func (s *Storage) writeBlob(data []byte) (string, error) {
	hash := computeHash(data)
	return hash, s.writeFile(hash, data, 0644)
}

func (s *Storage) readBlob(hash string) ([]byte, error) {
	file, err := s.openFile(hash, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}
	res := make([]byte, 1024)
	n, err := file.Read(res)
	if err != nil {
		return nil, err
	}
	return res[:n], nil
}

// internal

func (s *Storage) openFile(hash string, flag int, perm os.FileMode) (*os.File, error) {
	path := filepath.Join(s.Path, hash)
	return os.OpenFile(path, flag, perm)
}

func (s *Storage) writeFile(hash string, data []byte, perm os.FileMode) error {
	path := filepath.Join(s.Path, hash)
	return os.WriteFile(path, data, perm)
}

func (s *Storage) deleteFile(hash string) error {
	path := filepath.Join(s.Path, hash)
	return os.Remove(path)
}

func computeHash(data []byte) string {
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	return hash
}
