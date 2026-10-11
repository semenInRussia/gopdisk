package blobs

// blobs
//
// every blob is a real file in file system that responds on struct File that visible for user

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

var BlobsMaxSize int64 = 16 * 1024 * 1024 // 16MB
var ErrBlobOverflow = errors.New("probided data for one blob is more than expected")

type BlobStorage struct {
	Path string
}

type Blob struct {
	Hash string `json:"hash"`
	Size int64  `json:"size"`
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
	size := int64(len(data))
	blobs := make([]Blob, ComputeChunksAmount(size))

	i := 0
	errCh := make(chan error, ComputeChunksAmount(size))
	var wg sync.WaitGroup

	for arr := range SplitDataOnChunks(data) {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()
			bl, e := bs.AddOneChunk(arr)
			errCh <- e
			if e == nil {
				blobs[i] = *bl
			}
		}(i)

		i += 1
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return nil, err
		}
	}

	return blobs, nil
}

func SplitDataOnChunks(data []byte) chan []byte {
	ch := make(chan []byte)

	go func() {
		size := int64(len(data))
		var l int64 = 0
		for l < size {
			r := min(l+BlobsMaxSize, size)
			ch <- data[l:r]
			l = r
		}
		close(ch)
	}()

	return ch
}

func (bs *BlobStorage) AddOneChunk(data []byte) (*Blob, error) {
	size := int64(len(data))
	if size > BlobsMaxSize {
		return nil, ErrBlobOverflow
	}
	hash := computeHash(data)
	blob := Blob{Hash: hash, Size: size}
	return &blob, bs.writeFile(hash, data, 0644)
}

func (bs *BlobStorage) DeleteBlob(blob Blob) error {
	return bs.deleteFile(blob.Hash)
}

func (bs *BlobStorage) ListBlobs() ([]Blob, error) {
	entries, err := os.ReadDir(bs.Path)
	if err != nil {
		return nil, err
	}

	blobs := make([]Blob, 0, len(entries))

	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return nil, err
		}

		blobs = append(blobs, Blob{
			Hash: entry.Name(),
			Size: info.Size(),
		})
	}

	return blobs, nil
}

func (bs *BlobStorage) ReadBlobs(blobs []Blob) (<-chan []byte, <-chan error) {
	return bs.ReadBlobsContext(context.Background(), blobs)
}

func (bs *BlobStorage) ReadBlobsContext(ctx context.Context, blobs []Blob) (<-chan []byte, <-chan error) {
	data := make(chan []byte, 1)
	errCh := make(chan error, 1)

	go func() {
		defer close(data)
		defer close(errCh)

		for _, bl := range blobs {
			if err := ctx.Err(); err != nil {
				errCh <- err
				return
			}

			d, err := bs.ReadBlob(bl)
			if err != nil {
				errCh <- err
				return
			}

			select {
			case data <- d:
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}
		}
	}()

	return data, errCh
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

func ComputeChunksAmount(size int64) int64 {
	return (size + BlobsMaxSize - 1) / BlobsMaxSize
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
