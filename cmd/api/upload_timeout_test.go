package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Exercise actual socket deadlines, since ResponseRecorder cannot reproduce a
// slow phone sending a body beyond the server's ordinary read/write budgets.
func TestUploadPartAllowsSlowBody(t *testing.T) {
	app := &Application{}
	readStarted := make(chan struct{})
	readResult := make(chan error, 1)
	handler := app.uploadPartTimeout(func(w http.ResponseWriter, r *http.Request) {
		close(readStarted)
		body, err := io.ReadAll(r.Body)
		readResult <- err
		if err == nil {
			_, _ = w.Write(body)
		}
	})
	server := httptest.NewUnstartedServer(handler)
	server.Config.ReadTimeout = 50 * time.Millisecond
	server.Config.WriteTimeout = 50 * time.Millisecond
	server.Config.ReadHeaderTimeout = time.Second
	server.Start()
	defer server.Close()

	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	written := make(chan error, 1)
	go func() {
		select {
		case <-readStarted:
		case <-time.After(5 * time.Second):
			writer.CloseWithError(io.ErrClosedPipe)
			written <- io.ErrClosedPipe
			return
		}
		time.Sleep(150 * time.Millisecond)
		_, err := io.Copy(writer, strings.NewReader("slow upload part"))
		writer.CloseWithError(err)
		written <- err
	}()

	request, err := http.NewRequest(http.MethodPut, server.URL+"/v1/uploads/test/parts/0", reader)
	if err != nil {
		t.Fatal(err)
	}
	client := server.Client()
	client.Timeout = 5 * time.Second
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || string(body) != "slow upload part" {
		t.Fatalf("got status %d, body %q", response.StatusCode, body)
	}
	if err := <-written; err != nil {
		t.Fatalf("write body: %v", err)
	}
	if err := <-readResult; err != nil {
		t.Fatalf("read body: %v", err)
	}
}
