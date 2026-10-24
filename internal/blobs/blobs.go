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
	"strings"
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

func (bs *BlobStorage) AddData(ctx context.Context, data []byte) (*Blob, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if int64(len(data)) > BlobsMaxSize {
		return nil, ErrBlobOverflow
	}
	return bs.AddReader(ctx, bytes.NewReader(data))
}

// AddReader reads and stores at most one blob from reader, leaving any
// remaining data unread. It returns io.EOF without storing a blob when the
// reader has no data.
func (bs *BlobStorage) AddReader(ctx context.Context, reader io.Reader) (*Blob, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	temporary, err := os.CreateTemp(bs.Path, ".blob-*")
	if err != nil {
		return nil, err
	}
	defer temporary.Close()
	defer os.Remove(temporary.Name())

	hasher := sha256.New()
	size, err := io.Copy(
		io.MultiWriter(temporary, hasher),
		io.LimitReader(contextReader{ctx: ctx, reader: reader}, BlobsMaxSize),
	)
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, io.EOF
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := temporary.Chmod(0644); err != nil {
		return nil, err
	}
	if err := temporary.Close(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	hash := fmt.Sprintf("%x", hasher.Sum(nil))
	if err := os.Rename(temporary.Name(), filepath.Join(bs.Path, hash)); err != nil {
		return nil, err
	}

	return &Blob{Hash: hash, Size: size}, nil
}

func (bs *BlobStorage) DeleteBlob(ctx context.Context, blob Blob) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return bs.deleteFile(blob.Hash)
}

func (bs *BlobStorage) ListBlobs(ctx context.Context) ([]Blob, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(bs.Path)
	if err != nil {
		return nil, err
	}

	blobs := make([]Blob, 0, len(entries))

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// AddReader owns these files until they are renamed to their hash.
		// Exposing them to garbage collection can unlink an active upload.
		if strings.HasPrefix(entry.Name(), ".blob-") {
			continue
		}
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

func (bs *BlobStorage) ReadBlobs(ctx context.Context, blobs []Blob) (<-chan []byte, <-chan error) {
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

			d, err := bs.ReadBlob(ctx, bl)
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

func (bs *BlobStorage) ReadBlob(ctx context.Context, blob Blob) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	file, err := bs.openFile(blob.Hash, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	res := make([]byte, blob.Size)
	_, err = io.ReadFull(contextReader{ctx: ctx, reader: file}, res)
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

func (bs *BlobStorage) deleteFile(hash string) error {
	path := filepath.Join(bs.Path, hash)
	return os.Remove(path)
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(data)
}
