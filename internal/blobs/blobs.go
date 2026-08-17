package blobs

// blobs
//
// every blob is a real file in file system that responds on struct File that visible for user

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var BLOB_MAX_SIZE int = 16 * 1024 * 1024 // 16MB

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
	err := os.MkdirAll(path, 0755)
	return &s, err
}

// write data to correct file on file system, return hash of given data
func (bs *BlobStorage) AddData(data []byte) ([]Blob, error) {
	size := len(data)

	if size <= BLOB_MAX_SIZE {
		hash := computeHash(data)
		blob := Blob{Hash: hash, Size: size}
		return []Blob{blob}, bs.writeFile(hash, data, 0644)
	}
	l := 0
	blobs := []Blob{}
	for l < size {
		r := min(l+BLOB_MAX_SIZE, size)
		bl, err := bs.AddData(data[l:r])
		if err != nil {
			return nil, err
		}
		blobs = append(blobs, bl[0])
		l = r
	}
	return blobs, nil
}

func (bs *BlobStorage) DeleteBob(blob Blob) error {
	return bs.deleteFile(blob.Hash)
}

func (bs *BlobStorage) ReadBlobs(blobs []Blob) (<-chan []byte, <-chan error) {
	data := make(chan []byte)

	err := make(chan error)

	go func() {
		defer close(data)
		defer close(err)
		for _, bl := range blobs {
			d, e := bs.ReadBlob(bl)
			if e != nil {
				err <- e
				return
			}
			data <- d
		}
	}()

	return data, err
}

func (bs *BlobStorage) ReadBlob(blob Blob) ([]byte, error) {
	file, err := bs.openFile(blob.Hash, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	res := make([]byte, blob.Size)
	_, err = io.ReadFull(file, res)
	if err != nil {
		return nil, err
	}

	return res, nil
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
