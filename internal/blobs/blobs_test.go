package blobs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

func TestAddOneChunkReader(t *testing.T) {
	previousMaxSize := BlobsMaxSize
	BlobsMaxSize = 4
	t.Cleanup(func() { BlobsMaxSize = previousMaxSize })

	bs, err := MakeBlobStorage(t.TempDir())
	if err != nil {
		t.Fatalf("cannot create blob storage: %v", err)
	}

	if _, err := bs.AddReader(context.Background(), bytes.NewReader(nil)); !errors.Is(err, io.EOF) {
		t.Fatalf("AddOneChunkReader(empty) returned %v, expected %v", err, io.EOF)
	}
	listed, err := bs.ListBlobs(context.Background())
	if err != nil {
		t.Fatalf("cannot list blobs after empty reader: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("empty reader stored %d blobs, expected none", len(listed))
	}

	for _, data := range [][]byte{[]byte("abcd")} {
		blob, err := bs.AddReader(context.Background(), bytes.NewReader(data))
		if err != nil {
			t.Fatalf("AddOneChunkReader(%q) returned error: %v", data, err)
		}
		stored, err := bs.ReadBlob(context.Background(), *blob)
		if err != nil {
			t.Fatalf("cannot read stored blob: %v", err)
		}
		if !bytes.Equal(stored, data) {
			t.Fatalf("got stored data %q, expected %q", stored, data)
		}
	}

	if _, err := bs.AddData(context.Background(), []byte("abcde")); !errors.Is(err, ErrBlobOverflow) {
		t.Fatalf("AddOneChunk returned %v, expected %v", err, ErrBlobOverflow)
	}
	reader := bytes.NewReader([]byte("abcde"))
	limited, err := bs.AddReader(context.Background(), reader)
	if err != nil {
		t.Fatalf("AddOneChunkReader returned error for reader with remaining data: %v", err)
	}
	if limited.Size != BlobsMaxSize {
		t.Fatalf("stored blob has size %d, expected %d", limited.Size, BlobsMaxSize)
	}
	remaining, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("cannot read remaining reader data: %v", err)
	}
	if !bytes.Equal(remaining, []byte("e")) {
		t.Fatalf("got remaining data %q, expected %q", remaining, "e")
	}
	stored, err := bs.ReadBlob(context.Background(), *limited)
	if err != nil {
		t.Fatalf("cannot read limited blob: %v", err)
	}
	if !bytes.Equal(stored, []byte("abcd")) {
		t.Fatalf("got stored data %q, expected %q", stored, "abcd")
	}

	fromBytes, err := bs.AddData(context.Background(), []byte("abcd"))
	if err != nil {
		t.Fatalf("AddOneChunk returned error: %v", err)
	}
	fromReader, err := bs.AddReader(context.Background(), bytes.NewReader([]byte("abcd")))
	if err != nil {
		t.Fatalf("AddOneChunkReader returned error: %v", err)
	}
	if *fromBytes != *fromReader {
		t.Fatalf("AddOneChunk returned %+v, AddOneChunkReader returned %+v", fromBytes, fromReader)
	}
	assertNoTemporaryBlobs(t, bs.Path)
}

func TestAddOneChunkReaderReturnsReaderErrors(t *testing.T) {
	previousMaxSize := BlobsMaxSize
	BlobsMaxSize = 4
	t.Cleanup(func() { BlobsMaxSize = previousMaxSize })

	bs, err := MakeBlobStorage(t.TempDir())
	if err != nil {
		t.Fatalf("cannot create blob storage: %v", err)
	}
	expectedErr := errors.New("reader failed")

	if _, err := bs.AddReader(context.Background(), iotest.ErrReader(expectedErr)); !errors.Is(err, expectedErr) {
		t.Fatalf("AddOneChunkReader returned %v, expected %v", err, expectedErr)
	}
	assertNoTemporaryBlobs(t, bs.Path)
}

func TestBlobStorageOperationsUseCanceledContext(t *testing.T) {
	bs, err := MakeBlobStorage(t.TempDir())
	if err != nil {
		t.Fatalf("cannot create blob storage: %v", err)
	}
	stored, err := bs.AddData(context.Background(), []byte("data"))
	if err != nil {
		t.Fatalf("cannot prepare blob: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := bs.AddData(ctx, []byte("other")); !errors.Is(err, context.Canceled) {
		t.Errorf("AddData returned %v, expected %v", err, context.Canceled)
	}
	if _, err := bs.AddReader(ctx, bytes.NewReader([]byte("other"))); !errors.Is(err, context.Canceled) {
		t.Errorf("AddReader returned %v, expected %v", err, context.Canceled)
	}
	if _, err := bs.ListBlobs(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("ListBlobs returned %v, expected %v", err, context.Canceled)
	}
	if _, err := bs.ReadBlob(ctx, *stored); !errors.Is(err, context.Canceled) {
		t.Errorf("ReadBlob returned %v, expected %v", err, context.Canceled)
	}
	if err := bs.DeleteBlob(ctx, *stored); !errors.Is(err, context.Canceled) {
		t.Errorf("DeleteBlob returned %v, expected %v", err, context.Canceled)
	}

	dataCh, errCh := bs.ReadBlobs(ctx, []Blob{*stored})
	if data, ok := <-dataCh; ok {
		t.Errorf("ReadBlobs returned data %q for canceled context", data)
	}
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Errorf("ReadBlobs returned %v, expected %v", err, context.Canceled)
	}

	if _, err := bs.ReadBlob(context.Background(), *stored); err != nil {
		t.Fatalf("canceled DeleteBlob removed the blob: %v", err)
	}
	assertNoTemporaryBlobs(t, bs.Path)
}

func assertNoTemporaryBlobs(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatalf("cannot list blob storage: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".blob-") {
			t.Errorf("temporary blob %q was not cleaned up", entry.Name())
		}
	}
}

func TestReadBlobsReportsMissingBlobWithoutHanging(t *testing.T) {
	bs, err := MakeBlobStorage(t.TempDir())
	if err != nil {
		t.Fatalf("cannot create blob storage: %v", err)
	}

	dataCh, errCh := bs.ReadBlobs(context.Background(), []Blob{
		{Hash: "missing-blob", Size: 1},
	})

	select {
	case _, ok := <-dataCh:
		if ok {
			t.Fatal("data channel produced data for missing blob")
		}
	case <-time.After(time.Second):
		t.Fatal("data channel did not close after missing blob read error")
	}

	select {
	case err, ok := <-errCh:
		if !ok {
			t.Fatal("error channel closed without reporting missing blob error")
		}
		if err == nil {
			t.Fatal("got nil error for missing blob")
		}
	case <-time.After(time.Second):
		t.Fatal("error channel did not report missing blob error")
	}
}
