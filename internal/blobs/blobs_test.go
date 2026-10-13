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

func TestAddDataReaderStoresOrderedChunks(t *testing.T) {
	previousMaxSize := BlobsMaxSize
	BlobsMaxSize = 4
	t.Cleanup(func() { BlobsMaxSize = previousMaxSize })

	tests := []struct {
		name          string
		data          []byte
		expectedSizes []int64
	}{
		{name: "empty", data: nil, expectedSizes: []int64{}},
		{name: "smaller than one blob", data: []byte("abc"), expectedSizes: []int64{3}},
		{name: "exactly one blob", data: []byte("abcd"), expectedSizes: []int64{4}},
		{name: "multiple blobs", data: []byte("abcdefghi"), expectedSizes: []int64{4, 4, 1}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bs, err := MakeBlobStorage(t.TempDir())
			if err != nil {
				t.Fatalf("cannot create blob storage: %v", err)
			}

			stored, err := bs.AddDataReader(bytes.NewReader(test.data))
			if err != nil {
				t.Fatalf("AddDataReader returned error: %v", err)
			}
			if len(stored) != len(test.expectedSizes) {
				t.Fatalf("got %d blobs, expected %d", len(stored), len(test.expectedSizes))
			}

			var reconstructed []byte
			for i, blob := range stored {
				if blob.Size != test.expectedSizes[i] {
					t.Errorf("blob %d has size %d, expected %d", i, blob.Size, test.expectedSizes[i])
				}
				chunk, err := bs.ReadBlob(blob)
				if err != nil {
					t.Fatalf("cannot read blob %d: %v", i, err)
				}
				reconstructed = append(reconstructed, chunk...)
			}
			if !bytes.Equal(reconstructed, test.data) {
				t.Fatalf("got reconstructed data %q, expected %q", reconstructed, test.data)
			}
		})
	}
}

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

	if _, err := bs.AddOneChunkReader(bytes.NewReader([]byte("abcde"))); !errors.Is(err, ErrBlobOverflow) {
		t.Fatalf("got error %v, expected %v", err, ErrBlobOverflow)
	}
	if _, err := bs.AddOneChunk([]byte("abcde")); !errors.Is(err, ErrBlobOverflow) {
		t.Fatalf("AddOneChunk returned %v, expected %v", err, ErrBlobOverflow)
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

func TestAddReaderMethodsReturnReaderErrors(t *testing.T) {
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
	reader := io.MultiReader(bytes.NewReader([]byte("abcd")), iotest.ErrReader(expectedErr))
	if _, err := bs.AddDataReader(reader); !errors.Is(err, expectedErr) {
		t.Fatalf("AddDataReader returned %v, expected %v", err, expectedErr)
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
