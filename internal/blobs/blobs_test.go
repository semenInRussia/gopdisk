package blobs

import (
	"bytes"
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

	for _, data := range [][]byte{nil, []byte("abcd")} {
		blob, err := bs.AddOneChunkReader(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("AddOneChunkReader(%q) returned error: %v", data, err)
		}
		stored, err := bs.ReadBlob(*blob)
		if err != nil {
			t.Fatalf("cannot read stored blob: %v", err)
		}
		if !bytes.Equal(stored, data) {
			t.Fatalf("got stored data %q, expected %q", stored, data)
		}
	}

	if _, err := bs.AddOneChunk([]byte("abcde")); !errors.Is(err, ErrBlobOverflow) {
		t.Fatalf("AddOneChunk returned %v, expected %v", err, ErrBlobOverflow)
	}
	reader := bytes.NewReader([]byte("abcde"))
	limited, err := bs.AddOneChunkReader(reader)
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
	stored, err := bs.ReadBlob(*limited)
	if err != nil {
		t.Fatalf("cannot read limited blob: %v", err)
	}
	if !bytes.Equal(stored, []byte("abcd")) {
		t.Fatalf("got stored data %q, expected %q", stored, "abcd")
	}

	fromBytes, err := bs.AddOneChunk([]byte("abcd"))
	if err != nil {
		t.Fatalf("AddOneChunk returned error: %v", err)
	}
	fromReader, err := bs.AddOneChunkReader(bytes.NewReader([]byte("abcd")))
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

	if _, err := bs.AddOneChunkReader(iotest.ErrReader(expectedErr)); !errors.Is(err, expectedErr) {
		t.Fatalf("AddOneChunkReader returned %v, expected %v", err, expectedErr)
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

	dataCh, errCh := bs.ReadBlobs([]Blob{
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
