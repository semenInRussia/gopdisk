package blobs

import (
	"testing"
	"time"
)

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
