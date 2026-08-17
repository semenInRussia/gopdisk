package blobs

// blobs
//
// every blob is a real file in file system that responds on struct File that visible for user

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
)

type BlobStorage struct {
	Path string
}

type Blob struct {
	Hash string
	Size int
}

func MakeBlobStorage(path string) (*BlobStorage, error) {
	s := BlobStorage{
		Path: path,
	}
	err := os.Mkdir(path, 0755)
	if os.IsExist(err) || err == nil {
		return &s, nil
	}
	return nil, err
}

// write data to correct file on file system, return hash of given data
func (bs *BlobStorage) AddBlob(data []byte) (Blob, error) {
	hash := computeHash(data)
	blob := Blob{Hash: hash, Size: len(data)}
	return blob, bs.writeFile(hash, data, 0644)
}

func (bs *BlobStorage) DeleteBob(blob Blob) error {
	return bs.deleteFile(blob.Hash)
}

func (bs *BlobStorage) ReadBlob(blob Blob) ([]byte, error) {
	file, err := bs.openFile(blob.Hash, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}
	res := make([]byte, blob.Size)
	n, err := file.Read(res)
	if err != nil {
		return nil, err
	}
	return res[:n], nil
}

// internal

func (bs *BlobStorage) openFile(hash string, flag int, perm os.FileMode) (*os.File, error) {
	path := filepath.Join(bs.Path, hash)
	return os.OpenFile(path, flag, perm)
}

func (bs *BlobStorage) writeFile(hash string, data []byte, perm os.FileMode) error {
	path := filepath.Join(bs.Path, hash)
	return os.WriteFile(path, data, perm)
}

func (bs *BlobStorage) deleteFile(hash string) error {
	path := filepath.Join(bs.Path, hash)
	return os.Remove(path)
}

func computeHash(data []byte) string {
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	return hash
}
