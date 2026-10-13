package blobs

// blobs
//
// every blob is a real file in file system that responds on struct File that visible for user

import (
	"bytes"
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

// AddDataReader reads, splits, and stores all data from reader as ordered blobs.
func (bs *BlobStorage) AddDataReader(reader io.Reader) ([]Blob, error) {
	stored := make([]Blob, 0)
	for {
		chunk := make([]byte, BlobsMaxSize)
		size, err := io.ReadFull(reader, chunk)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, err
		}

		blob, storeErr := bs.AddOneChunk(chunk[:size])
		if storeErr != nil {
			return nil, storeErr
		}
		stored = append(stored, *blob)
		if errors.Is(err, io.ErrUnexpectedEOF) {
			break
		}
	}
	return stored, nil
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
	return bs.AddOneChunkReader(bytes.NewReader(data))
}

// AddOneChunkReader reads and stores at most one blob from reader.
func (bs *BlobStorage) AddOneChunkReader(reader io.Reader) (*Blob, error) {
	temporary, err := os.CreateTemp(bs.Path, ".blob-*")
	if err != nil {
		return nil, err
	}
	defer temporary.Close()
	defer os.Remove(temporary.Name())

	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(temporary, hasher), io.LimitReader(reader, BlobsMaxSize+1))
	if err != nil {
		return nil, err
	}
	if size > BlobsMaxSize {
		return nil, ErrBlobOverflow
	}
	if err := temporary.Chmod(0644); err != nil {
		return nil, err
	}
	if err := temporary.Close(); err != nil {
		return nil, err
	}

	hash := fmt.Sprintf("%x", hasher.Sum(nil))
	if err := os.Rename(temporary.Name(), filepath.Join(bs.Path, hash)); err != nil {
		return nil, err
	}

	return &Blob{Hash: hash, Size: size}, nil
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

func (bs *BlobStorage) deleteFile(hash string) error {
	path := filepath.Join(bs.Path, hash)
	return os.Remove(path)
}
