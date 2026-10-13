package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"
	"time"

	"gopherdisk/internal/blobs"
	"gopherdisk/internal/data"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/julienschmidt/httprouter"
)

func newTestApplication(t *testing.T) (*Application, sqlmock.Sqlmock) {
	t.Helper()

	app, mock, _ := newTestApplicationWithStoragePath(t)
	return app, mock
}

func newTestApplicationWithStoragePath(t *testing.T) (*Application, sqlmock.Sqlmock, string) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("cannot create mock database: %v", err)
	}
	t.Cleanup(func() {
		mock.ExpectClose()
		if err := db.Close(); err != nil {
			t.Errorf("cannot close mock database: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet database expectations: %v", err)
		}
	})

	storagePath := t.TempDir()
	storage, err := data.MakeStorage(storagePath, db)
	if err != nil {
		t.Fatalf("cannot create test storage: %v", err)
	}

	return &Application{
		storage: storage,
		logger:  log.New(io.Discard, "", 0),
	}, mock, storagePath
}

func serveRequest(t *testing.T, app *Application, method, target string, body []byte) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	app.routes().ServeHTTP(recorder, req)

	return recorder
}

func serveFilesUploadsPatchHandler(t *testing.T, app *Application, fileID string, body []byte) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPatch, "/v1/files/"+fileID, bytes.NewReader(body))
	params := httprouter.Params{{Key: "id", Value: fileID}}
	req = req.WithContext(context.WithValue(req.Context(), httprouter.ParamsKey, params))
	recorder := httptest.NewRecorder()
	app.filesPatchHandler(recorder, req)

	return recorder
}

func serveFilesDeleteHandler(t *testing.T, app *Application, fileID string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodDelete, "/v1/files/"+fileID, nil)
	params := httprouter.Params{{Key: "id", Value: fileID}}
	req = req.WithContext(context.WithValue(req.Context(), httprouter.ParamsKey, params))
	recorder := httptest.NewRecorder()
	app.filesDeleteHandler(recorder, req)

	return recorder
}

func decodeResponse[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()

	var response T
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("cannot decode response body %q: %v", recorder.Body.String(), err)
	}
	return response
}

func assertJSONResponse(t *testing.T, recorder *httptest.ResponseRecorder, status int) {
	t.Helper()

	if recorder.Code != status {
		t.Fatalf("got status %d, expected %d; body: %s", recorder.Code, status, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("got Content-Type %q, expected %q", got, "application/json")
	}
}

func TestHealthCheckEndpoint(t *testing.T) {
	app, _ := newTestApplication(t)

	recorder := serveRequest(t, app, http.MethodGet, "/v1/healthcheck", nil)

	assertJSONResponse(t, recorder, http.StatusNonAuthoritativeInfo)
	response := decodeResponse[struct {
		Status  string    `json:"status"`
		Version string    `json:"version"`
		Time    time.Time `json:"time"`
	}](t, recorder)

	if response.Status != "available" {
		t.Errorf("got status value %q, expected %q", response.Status, "available")
	}
	if response.Version != version {
		t.Errorf("got version %q, expected %q", response.Version, version)
	}
	if response.Time.IsZero() {
		t.Error("response time must not be zero")
	}
}

func TestUsersPostEndpoint(t *testing.T) {
	t.Run("creates user", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO users (id, name) VALUES ($1, $2)")).
			WithArgs(sqlmock.AnyArg(), "Kotik").
			WillReturnResult(sqlmock.NewResult(0, 1))

		recorder := serveRequest(t, app, http.MethodPost, "/v1/users", []byte(`{"name":"Kotik"}`))

		assertJSONResponse(t, recorder, http.StatusCreated)
		response := decodeResponse[struct {
			User data.User `json:"user"`
		}](t, recorder)
		if response.User.Name != "Kotik" {
			t.Errorf("got user name %q, expected %q", response.User.Name, "Kotik")
		}
		if _, err := uuid.Parse(string(response.User.ID)); err != nil {
			t.Errorf("got invalid user UUID %q: %v", response.User.ID, err)
		}
	})

	t.Run("rejects malformed JSON", func(t *testing.T) {
		app, _ := newTestApplication(t)

		recorder := serveRequest(t, app, http.MethodPost, "/v1/users", []byte(`{"name":`))

		assertJSONResponse(t, recorder, http.StatusBadRequest)
	})

	t.Run("rejects empty name", func(t *testing.T) {
		app, _ := newTestApplication(t)

		recorder := serveRequest(t, app, http.MethodPost, "/v1/users", []byte(`{"name":""}`))

		assertJSONResponse(t, recorder, http.StatusUnprocessableEntity)
		response := decodeResponse[struct {
			Error map[string]string `json:"error"`
		}](t, recorder)
		if response.Error["user"] == "" {
			t.Error("expected a user-name validation error")
		}
	})

	t.Run("returns bad request on insert error", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO users (id, name) VALUES ($1, $2)")).
			WithArgs(sqlmock.AnyArg(), "Kotik").
			WillReturnError(errors.New("insert failed"))

		recorder := serveRequest(t, app, http.MethodPost, "/v1/users", []byte(`{"name":"Kotik"}`))

		assertJSONResponse(t, recorder, http.StatusBadRequest)
	})
}

func TestUsersGetEndpoint(t *testing.T) {
	const userID = "baab6197-3e2d-4969-8492-247bbba900dc"
	query := regexp.QuoteMeta("SELECT id, name FROM users WHERE id = $1")

	t.Run("returns user", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(query).
			WithArgs(data.UserID(userID)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(userID, "Kotik"))

		recorder := serveRequest(t, app, http.MethodGet, "/v1/users/"+userID, nil)

		assertJSONResponse(t, recorder, http.StatusOK)
		response := decodeResponse[struct {
			User data.User `json:"user"`
		}](t, recorder)
		if response.User.ID != data.UserID(userID) || response.User.Name != "Kotik" {
			t.Errorf("got user %+v, expected id %q and name %q", response.User, userID, "Kotik")
		}
	})

	t.Run("returns not found", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(query).
			WithArgs(data.UserID(userID)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}))

		recorder := serveRequest(t, app, http.MethodGet, "/v1/users/"+userID, nil)

		assertJSONResponse(t, recorder, http.StatusNotFound)
	})

	t.Run("returns server error", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(query).
			WithArgs(data.UserID(userID)).
			WillReturnError(errors.New("select failed"))

		recorder := serveRequest(t, app, http.MethodGet, "/v1/users/"+userID, nil)

		assertJSONResponse(t, recorder, http.StatusInternalServerError)
	})
}

func TestUsersDeleteEndpoint(t *testing.T) {
	const userID = "baab6197-3e2d-4969-8492-247bbba900dc"
	query := regexp.QuoteMeta("DELETE FROM users WHERE id = $1")

	t.Run("deletes user", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectExec(query).
			WithArgs(data.UserID(userID)).
			WillReturnResult(sqlmock.NewResult(0, 1))

		recorder := serveRequest(t, app, http.MethodDelete, "/v1/users/"+userID, nil)

		if recorder.Code != http.StatusNoContent {
			t.Errorf("got status %d, expected %d; body: %s", recorder.Code, http.StatusNoContent, recorder.Body.String())
		}
		if recorder.Body.Len() != 0 {
			t.Errorf("got response body %q, expected an empty body", recorder.Body.String())
		}
	})

	t.Run("returns not found", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectExec(query).
			WithArgs(data.UserID(userID)).
			WillReturnResult(sqlmock.NewResult(0, 0))

		recorder := serveRequest(t, app, http.MethodDelete, "/v1/users/"+userID, nil)

		assertJSONResponse(t, recorder, http.StatusNotFound)
	})

	t.Run("returns server error", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectExec(query).
			WithArgs(data.UserID(userID)).
			WillReturnResult(sqlmock.NewErrorResult(errors.New("delete failed")))

		recorder := serveRequest(t, app, http.MethodDelete, "/v1/users/"+userID, nil)

		assertJSONResponse(t, recorder, http.StatusInternalServerError)
	})
}

func TestUsersFilesGetEndpoint(t *testing.T) {
	const (
		userID      = "baab6197-3e2d-4969-8492-247bbba900dc"
		firstFileID = "32f11d17-b1f5-4fb0-94ca-614903665dff"
		lastFileID  = "fb4e2882-0c27-4cf5-b17e-e84fb8a05e5c"
	)
	userQuery := regexp.QuoteMeta("SELECT id, name FROM users WHERE id = $1")
	filesQuery := regexp.QuoteMeta(`
		SELECT id, name, filetype, owner_id, status, size
		FROM files
		WHERE owner_id = $1 AND status = $2
		ORDER BY name, id`)

	t.Run("returns ready files in order", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(userQuery).
			WithArgs(data.UserID(userID)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(userID, "Kotik"))
		mock.ExpectQuery(filesQuery).
			WithArgs(data.UserID(userID), data.FileStatusReady).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "filetype", "owner_id", "status", "size"}).
				AddRow(firstFileID, "alpha.txt", "TXT", userID, data.FileStatusReady, int64(5)).
				AddRow(lastFileID, "movie.mov", "MOV", userID, data.FileStatusReady, int64(1024)))

		recorder := serveRequest(t, app, http.MethodGet, "/v1/users/"+userID+"/files", nil)

		assertJSONResponse(t, recorder, http.StatusOK)
		response := decodeResponse[struct {
			Files []data.File `json:"files"`
		}](t, recorder)
		expected := []data.File{
			{ID: data.FileID(firstFileID), Name: "alpha.txt", Type: data.FileType("TXT"), OwnerID: data.UserID(userID), Status: data.FileStatusReady, Size: 5},
			{ID: data.FileID(lastFileID), Name: "movie.mov", Type: data.FileType("MOV"), OwnerID: data.UserID(userID), Status: data.FileStatusReady, Size: 1024},
		}
		if len(response.Files) != len(expected) {
			t.Fatalf("got %d files, expected %d", len(response.Files), len(expected))
		}
		for i := range expected {
			if response.Files[i] != expected[i] {
				t.Errorf("got file %d %+v, expected %+v", i, response.Files[i], expected[i])
			}
		}
	})

	t.Run("returns empty list", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(userQuery).
			WithArgs(data.UserID(userID)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(userID, "Kotik"))
		mock.ExpectQuery(filesQuery).
			WithArgs(data.UserID(userID), data.FileStatusReady).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "filetype", "owner_id", "status", "size"}))

		recorder := serveRequest(t, app, http.MethodGet, "/v1/users/"+userID+"/files", nil)

		assertJSONResponse(t, recorder, http.StatusOK)
		response := decodeResponse[struct {
			Files []data.File `json:"files"`
		}](t, recorder)
		if response.Files == nil {
			t.Fatal("got null files, expected an empty array")
		}
		if len(response.Files) != 0 {
			t.Errorf("got %d files, expected none", len(response.Files))
		}
	})

	t.Run("returns bad request for missing user", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(userQuery).
			WithArgs(data.UserID(userID)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}))

		recorder := serveRequest(t, app, http.MethodGet, "/v1/users/"+userID+"/files", nil)

		assertJSONResponse(t, recorder, http.StatusBadRequest)
	})

	t.Run("returns server error", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(userQuery).
			WithArgs(data.UserID(userID)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(userID, "Kotik"))
		mock.ExpectQuery(filesQuery).
			WithArgs(data.UserID(userID), data.FileStatusReady).
			WillReturnError(errors.New("select failed"))

		recorder := serveRequest(t, app, http.MethodGet, "/v1/users/"+userID+"/files", nil)

		assertJSONResponse(t, recorder, http.StatusInternalServerError)
	})
}

func TestUsersUploadsGetEndpoint(t *testing.T) {
	const (
		userID      = "baab6197-3e2d-4969-8492-247bbba900dc"
		firstFileID = "32f11d17-b1f5-4fb0-94ca-614903665dff"
		lastFileID  = "fb4e2882-0c27-4cf5-b17e-e84fb8a05e5c"
	)
	userQuery := regexp.QuoteMeta("SELECT id, name FROM users WHERE id = $1")
	uploadsQuery := regexp.QuoteMeta(`
		SELECT id, name, filetype, owner_id, status, size
		FROM files
		WHERE owner_id = $1 AND status = $2
		ORDER BY name, id`)

	t.Run("returns uploads in order", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(userQuery).
			WithArgs(data.UserID(userID)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(userID, "Kotik"))
		mock.ExpectQuery(uploadsQuery).
			WithArgs(data.UserID(userID), data.FileStatusUploading).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "filetype", "owner_id", "status", "size"}).
				AddRow(firstFileID, "alpha.txt", "TXT", userID, data.FileStatusUploading, int64(5)).
				AddRow(lastFileID, "movie.mov", "MOV", userID, data.FileStatusUploading, int64(1024)))

		recorder := serveRequest(t, app, http.MethodGet, "/v1/users/"+userID+"/uploads", nil)

		assertJSONResponse(t, recorder, http.StatusOK)
		response := decodeResponse[struct {
			Files []data.File `json:"files"`
		}](t, recorder)
		expected := []data.File{
			{ID: data.FileID(firstFileID), Name: "alpha.txt", Type: data.FileType("TXT"), OwnerID: data.UserID(userID), Status: data.FileStatusUploading, Size: 5},
			{ID: data.FileID(lastFileID), Name: "movie.mov", Type: data.FileType("MOV"), OwnerID: data.UserID(userID), Status: data.FileStatusUploading, Size: 1024},
		}
		if len(response.Files) != len(expected) {
			t.Fatalf("got %d uploads, expected %d", len(response.Files), len(expected))
		}
		for i := range expected {
			if response.Files[i] != expected[i] {
				t.Errorf("got upload %d %+v, expected %+v", i, response.Files[i], expected[i])
			}
		}
	})

	t.Run("returns empty list", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(userQuery).
			WithArgs(data.UserID(userID)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(userID, "Kotik"))
		mock.ExpectQuery(uploadsQuery).
			WithArgs(data.UserID(userID), data.FileStatusUploading).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "filetype", "owner_id", "status", "size"}))

		recorder := serveRequest(t, app, http.MethodGet, "/v1/users/"+userID+"/uploads", nil)

		assertJSONResponse(t, recorder, http.StatusOK)
		response := decodeResponse[struct {
			Files []data.File `json:"files"`
		}](t, recorder)
		if response.Files == nil {
			t.Fatal("got null files, expected an empty array")
		}
		if len(response.Files) != 0 {
			t.Errorf("got %d uploads, expected none", len(response.Files))
		}
	})

	t.Run("returns bad request for missing user", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(userQuery).
			WithArgs(data.UserID(userID)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}))

		recorder := serveRequest(t, app, http.MethodGet, "/v1/users/"+userID+"/uploads", nil)

		assertJSONResponse(t, recorder, http.StatusBadRequest)
	})

	t.Run("returns server error", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(userQuery).
			WithArgs(data.UserID(userID)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(userID, "Kotik"))
		mock.ExpectQuery(uploadsQuery).
			WithArgs(data.UserID(userID), data.FileStatusUploading).
			WillReturnError(errors.New("select failed"))

		recorder := serveRequest(t, app, http.MethodGet, "/v1/users/"+userID+"/uploads", nil)

		assertJSONResponse(t, recorder, http.StatusInternalServerError)
	})
}

func TestFilesGetEndpoint(t *testing.T) {
	const (
		fileID  = "32f11d17-b1f5-4fb0-94ca-614903665dff"
		ownerID = "baab6197-3e2d-4969-8492-247bbba900dc"
	)
	query := regexp.QuoteMeta("SELECT id, name, filetype, owner_id, status, size FROM files WHERE id = $1 AND status = $2")

	t.Run("returns ready file", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(query).
			WithArgs(data.FileID(fileID), data.FileStatusReady).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "filetype", "owner_id", "status", "size"}).
				AddRow(fileID, "movie.mov", "MOV", ownerID, data.FileStatusReady, int64(1024)))

		recorder := serveRequest(t, app, http.MethodGet, "/v1/files/"+fileID, nil)

		assertJSONResponse(t, recorder, http.StatusOK)
		response := decodeResponse[struct {
			File data.File `json:"file"`
		}](t, recorder)
		if response.File.ID != data.FileID(fileID) || response.File.Status != data.FileStatusReady {
			t.Errorf("got file %+v, expected ready file %q", response.File, fileID)
		}
	})

	t.Run("returns not found", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(query).
			WithArgs(data.FileID(fileID), data.FileStatusReady).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "filetype", "owner_id", "status", "size"}))

		recorder := serveRequest(t, app, http.MethodGet, "/v1/files/"+fileID, nil)

		assertJSONResponse(t, recorder, http.StatusNotFound)
	})

	t.Run("returns server error", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(query).
			WithArgs(data.FileID(fileID), data.FileStatusReady).
			WillReturnError(errors.New("select failed"))

		recorder := serveRequest(t, app, http.MethodGet, "/v1/files/"+fileID, nil)

		assertJSONResponse(t, recorder, http.StatusInternalServerError)
	})
}

func TestFilesUploadsPatchEndpoint(t *testing.T) {
	const (
		fileID  = "32f11d17-b1f5-4fb0-94ca-614903665dff"
		ownerID = "baab6197-3e2d-4969-8492-247bbba900dc"
	)
	selectQuery := regexp.QuoteMeta("SELECT id, name, filetype, owner_id, status, size FROM files WHERE id = $1 AND status = $2")
	updateQuery := regexp.QuoteMeta("UPDATE files SET name = $1 WHERE id = $2 AND status = $3")
	body := []byte(`{"name":"holiday.mov"}`)
	fileColumns := []string{"id", "name", "filetype", "owner_id", "status", "size"}

	expectFile := func(mock sqlmock.Sqlmock) {
		mock.ExpectQuery(selectQuery).
			WithArgs(data.FileID(fileID), data.FileStatusReady).
			WillReturnRows(sqlmock.NewRows(fileColumns).
				AddRow(fileID, "movie.mov", "MOV", ownerID, data.FileStatusReady, int64(1024)))
	}

	t.Run("routes documented file path", func(t *testing.T) {
		app, mock := newTestApplication(t)
		expectFile(mock)
		mock.ExpectExec(updateQuery).
			WithArgs("holiday.mov", data.FileID(fileID), data.FileStatusReady).
			WillReturnResult(sqlmock.NewResult(0, 1))

		recorder := serveRequest(t, app, http.MethodPatch, "/v1/files/"+fileID, body)

		assertJSONResponse(t, recorder, http.StatusOK)
	})

	t.Run("renames file", func(t *testing.T) {
		app, mock := newTestApplication(t)
		expectFile(mock)
		mock.ExpectExec(updateQuery).
			WithArgs("holiday.mov", data.FileID(fileID), data.FileStatusReady).
			WillReturnResult(sqlmock.NewResult(0, 1))

		recorder := serveFilesUploadsPatchHandler(t, app, fileID, body)

		assertJSONResponse(t, recorder, http.StatusOK)
		response := decodeResponse[struct {
			File data.File `json:"file"`
		}](t, recorder)
		if response.File.ID != data.FileID(fileID) {
			t.Errorf("got file ID %q, expected %q", response.File.ID, fileID)
		}
		if response.File.Name != "holiday.mov" {
			t.Errorf("got file name %q, expected %q", response.File.Name, "holiday.mov")
		}
	})

	t.Run("rejects malformed JSON", func(t *testing.T) {
		app, _ := newTestApplication(t)

		recorder := serveFilesUploadsPatchHandler(t, app, fileID, []byte(`{"name":`))

		assertJSONResponse(t, recorder, http.StatusBadRequest)
	})

	t.Run("rejects empty name", func(t *testing.T) {
		app, _ := newTestApplication(t)

		recorder := serveFilesUploadsPatchHandler(t, app, fileID, []byte(`{"name":""}`))

		assertJSONResponse(t, recorder, http.StatusUnprocessableEntity)
		response := decodeResponse[struct {
			Error map[string]string `json:"error"`
		}](t, recorder)
		if response.Error["user"] != "user's name cannot be empty" {
			t.Errorf("got validation error %q, expected %q", response.Error["user"], "user's name cannot be empty")
		}
	})

	t.Run("returns not found", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(selectQuery).
			WithArgs(data.FileID(fileID), data.FileStatusReady).
			WillReturnRows(sqlmock.NewRows(fileColumns))

		recorder := serveFilesUploadsPatchHandler(t, app, fileID, body)

		assertJSONResponse(t, recorder, http.StatusNotFound)
	})

	t.Run("returns server error on lookup failure", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(selectQuery).
			WithArgs(data.FileID(fileID), data.FileStatusReady).
			WillReturnError(errors.New("select failed"))

		recorder := serveFilesUploadsPatchHandler(t, app, fileID, body)

		assertJSONResponse(t, recorder, http.StatusInternalServerError)
	})

	t.Run("returns server error on update failure", func(t *testing.T) {
		app, mock := newTestApplication(t)
		expectFile(mock)
		mock.ExpectExec(updateQuery).
			WithArgs("holiday.mov", data.FileID(fileID), data.FileStatusReady).
			WillReturnError(errors.New("update failed"))

		recorder := serveFilesUploadsPatchHandler(t, app, fileID, body)

		assertJSONResponse(t, recorder, http.StatusInternalServerError)
	})
}

func TestFilesDeleteHandler(t *testing.T) {
	const (
		fileID  = "32f11d17-b1f5-4fb0-94ca-614903665dff"
		ownerID = "baab6197-3e2d-4969-8492-247bbba900dc"
	)
	selectQuery := regexp.QuoteMeta("SELECT id, name, filetype, owner_id, status, size FROM files WHERE id = $1")
	deleteQuery := regexp.QuoteMeta("DELETE FROM files WHERE id = $1")
	fileColumns := []string{"id", "name", "filetype", "owner_id", "status", "size"}

	expectFile := func(mock sqlmock.Sqlmock) {
		mock.ExpectQuery(selectQuery).
			WithArgs(data.FileID(fileID)).
			WillReturnRows(sqlmock.NewRows(fileColumns).
				AddRow(fileID, "movie.mov", "MOV", ownerID, data.FileStatusReady, int64(1024)))
	}

	t.Run("deletes logical file", func(t *testing.T) {
		app, mock := newTestApplication(t)
		expectFile(mock)
		mock.ExpectExec(deleteQuery).
			WithArgs(data.FileID(fileID)).
			WillReturnResult(sqlmock.NewResult(0, 1))

		recorder := serveFilesDeleteHandler(t, app, fileID)

		if recorder.Code != http.StatusNoContent {
			t.Errorf("got status %d, expected %d; body: %s", recorder.Code, http.StatusNoContent, recorder.Body.String())
		}
		if recorder.Body.Len() != 0 {
			t.Errorf("got response body %q, expected an empty body", recorder.Body.String())
		}
	})

	t.Run("returns not found", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(selectQuery).
			WithArgs(data.FileID(fileID)).
			WillReturnRows(sqlmock.NewRows(fileColumns))

		recorder := serveFilesDeleteHandler(t, app, fileID)

		assertJSONResponse(t, recorder, http.StatusNotFound)
	})

	t.Run("returns server error on lookup failure", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(selectQuery).
			WithArgs(data.FileID(fileID)).
			WillReturnError(errors.New("select failed"))

		recorder := serveFilesDeleteHandler(t, app, fileID)

		assertJSONResponse(t, recorder, http.StatusInternalServerError)
	})

	t.Run("returns server error on delete failure", func(t *testing.T) {
		app, mock := newTestApplication(t)
		expectFile(mock)
		mock.ExpectExec(deleteQuery).
			WithArgs(data.FileID(fileID)).
			WillReturnError(errors.New("delete failed"))

		recorder := serveFilesDeleteHandler(t, app, fileID)

		assertJSONResponse(t, recorder, http.StatusInternalServerError)
	})
}

func TestFilesUploadPostEndpoint(t *testing.T) {
	const (
		fileID  = "32f11d17-b1f5-4fb0-94ca-614903665dff"
		ownerID = "baab6197-3e2d-4969-8492-247bbba900dc"
	)
	query := regexp.QuoteMeta(`
		INSERT INTO files (id, name, owner_id, filetype, size, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`)
	body := []byte(`{"owner_id":"` + ownerID + `","name":"movie.mov","type":"MOV","size":1024}`)

	t.Run("initializes upload", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(query).
			WithArgs(sqlmock.AnyArg(), "movie.mov", data.UserID(ownerID), data.FileType("MOV"), int64(1024), data.FileStatusUploading).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(fileID))

		recorder := serveRequest(t, app, http.MethodPost, "/v1/files/upload", body)

		assertJSONResponse(t, recorder, http.StatusOK)
		response := decodeResponse[struct {
			File data.File `json:"file"`
		}](t, recorder)
		if response.File.ID != data.FileID(fileID) {
			t.Errorf("got file ID %q, expected %q", response.File.ID, fileID)
		}
		if response.File.Status != data.FileStatusUploading {
			t.Errorf("got file status %q, expected %q", response.File.Status, data.FileStatusUploading)
		}
	})

	t.Run("rejects malformed JSON", func(t *testing.T) {
		app, _ := newTestApplication(t)

		recorder := serveRequest(t, app, http.MethodPost, "/v1/files/upload", []byte(`{"owner_id":`))

		assertJSONResponse(t, recorder, http.StatusBadRequest)
	})

	t.Run("returns server error on insert failure", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(query).WillReturnError(errors.New("insert failed"))

		recorder := serveRequest(t, app, http.MethodPost, "/v1/files/upload", body)

		assertJSONResponse(t, recorder, http.StatusInternalServerError)
	})
}

func TestFilesUploadGetEndpoint(t *testing.T) {
	const (
		fileID  = "32f11d17-b1f5-4fb0-94ca-614903665dff"
		ownerID = "baab6197-3e2d-4969-8492-247bbba900dc"
	)
	query := regexp.QuoteMeta(`
		SELECT f.id, f.owner_id, f.name, f.filetype, f.status, f.size, fb.position, b.size
		FROM files f
		LEFT JOIN file_blobs fb ON fb.file_id = f.id
		LEFT JOIN blobs b ON b.hash = fb.blob_hash
		WHERE f.id = $1 AND f.status = $2
		ORDER BY fb.position
	`)
	columns := []string{"id", "owner_id", "name", "filetype", "status", "file_size", "position", "blob_size"}

	t.Run("returns upload progress with ordered parts", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(query).
			WithArgs(data.FileID(fileID), data.FileStatusUploading).
			WillReturnRows(sqlmock.NewRows(columns).
				AddRow(fileID, ownerID, "movie.mov", "video/quicktime", data.FileStatusUploading, int64(40), int64(0), int64(16)).
				AddRow(fileID, ownerID, "movie.mov", "video/quicktime", data.FileStatusUploading, int64(40), int64(1), int64(16)))

		recorder := serveRequest(t, app, http.MethodGet, "/v1/uploads/"+fileID, nil)

		assertJSONResponse(t, recorder, http.StatusOK)
		response := decodeResponse[struct {
			Upload data.Upload `json:"upload"`
		}](t, recorder)
		if response.Upload.FileID != data.FileID(fileID) {
			t.Errorf("got file ID %q, expected %q", response.Upload.FileID, fileID)
		}
		if response.Upload.Type != data.FileType("video/quicktime") {
			t.Errorf("got file type %q, expected %q", response.Upload.Type, "video/quicktime")
		}
		if response.Upload.ReceivedSize != 32 {
			t.Errorf("got received size %d, expected 32", response.Upload.ReceivedSize)
		}
		expectedParts := []data.UploadPart{{Position: 0, Size: 16}, {Position: 1, Size: 16}}
		if len(response.Upload.Parts) != len(expectedParts) {
			t.Fatalf("got %d parts, expected %d", len(response.Upload.Parts), len(expectedParts))
		}
		for i := range expectedParts {
			if response.Upload.Parts[i] != expectedParts[i] {
				t.Errorf("got part %d %+v, expected %+v", i, response.Upload.Parts[i], expectedParts[i])
			}
		}
	})

	t.Run("returns an empty parts array", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(query).
			WithArgs(data.FileID(fileID), data.FileStatusUploading).
			WillReturnRows(sqlmock.NewRows(columns).
				AddRow(fileID, ownerID, "empty.txt", "text/plain", data.FileStatusUploading, int64(0), nil, nil))

		recorder := serveRequest(t, app, http.MethodGet, "/v1/uploads/"+fileID, nil)

		assertJSONResponse(t, recorder, http.StatusOK)
		response := decodeResponse[struct {
			Upload data.Upload `json:"upload"`
		}](t, recorder)
		if response.Upload.Parts == nil {
			t.Fatal("got null parts, expected an empty array")
		}
		if len(response.Upload.Parts) != 0 {
			t.Errorf("got %d parts, expected none", len(response.Upload.Parts))
		}
		if response.Upload.ReceivedSize != 0 {
			t.Errorf("got received size %d, expected 0", response.Upload.ReceivedSize)
		}
	})

	t.Run("returns not found for a missing or completed upload", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(query).
			WithArgs(data.FileID(fileID), data.FileStatusUploading).
			WillReturnRows(sqlmock.NewRows(columns))

		recorder := serveRequest(t, app, http.MethodGet, "/v1/uploads/"+fileID, nil)

		assertJSONResponse(t, recorder, http.StatusNotFound)
	})

	t.Run("rejects an invalid file ID", func(t *testing.T) {
		app, _ := newTestApplication(t)

		recorder := serveRequest(t, app, http.MethodGet, "/v1/uploads/not-a-uuid", nil)

		assertJSONResponse(t, recorder, http.StatusBadRequest)
	})

	t.Run("returns server error on query failure", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(query).
			WithArgs(data.FileID(fileID), data.FileStatusUploading).
			WillReturnError(errors.New("select failed"))

		recorder := serveRequest(t, app, http.MethodGet, "/v1/uploads/"+fileID, nil)

		assertJSONResponse(t, recorder, http.StatusInternalServerError)
	})
}

func TestUploadsPartsPutEndpoint(t *testing.T) {
	const fileID = "32f11d17-b1f5-4fb0-94ca-614903665dff"
	const position = 2

	getBlobQuery := regexp.QuoteMeta(`
		SELECT b.hash, b.size
		FROM files f
		LEFT JOIN file_blobs fb ON fb.file_id = f.id
		LEFT JOIN blobs b ON b.hash = fb.blob_hash
		WHERE f.id = $1 and fb.position = $2
	`)
	lockUploadQuery := regexp.QuoteMeta("SELECT TRUE FROM files WHERE id = $1 AND status = $2 FOR UPDATE")
	saveBlobQuery := regexp.QuoteMeta("INSERT INTO blobs (hash, size) VALUES ($1, $2) ON CONFLICT (hash) DO NOTHING")
	putBlobQuery := regexp.QuoteMeta(`
		INSERT INTO file_blobs (file_id, blob_hash, position)
		VALUES ($1, $2, $3)
		ON CONFLICT (file_id, position)
		DO UPDATE SET blob_hash = EXCLUDED.blob_hash`)
	blobColumns := []string{"hash", "size"}

	expectBlobLookup := func(mock sqlmock.Sqlmock, found bool) {
		rows := sqlmock.NewRows(blobColumns)
		if found {
			rows.AddRow("previous-hash", int64(3))
		}
		mock.ExpectQuery(getBlobQuery).
			WithArgs(data.FileID(fileID), position).
			WillReturnRows(rows)
	}

	expectPartStored := func(mock sqlmock.Sqlmock, body []byte) {
		hash := fmt.Sprintf("%x", sha256.Sum256(body))
		mock.ExpectBegin()
		mock.ExpectQuery(lockUploadQuery).
			WithArgs(data.FileID(fileID), data.FileStatusUploading).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectExec(saveBlobQuery).
			WithArgs(hash, int64(len(body))).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(putBlobQuery).
			WithArgs(data.FileID(fileID), hash, position).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
	}

	assertPartResponse := func(t *testing.T, recorder *httptest.ResponseRecorder, status int, size int64) {
		t.Helper()
		assertJSONResponse(t, recorder, status)
		response := decodeResponse[struct {
			Part data.UploadPart `json:"part"`
		}](t, recorder)
		if response.Part.Position != position || response.Part.Size != size {
			t.Errorf("got part %+v, expected position %d and size %d", response.Part, position, size)
		}
	}

	t.Run("creates a new part", func(t *testing.T) {
		app, mock := newTestApplication(t)
		body := []byte("new part")
		expectBlobLookup(mock, false)
		expectPartStored(mock, body)

		recorder := serveRequest(t, app, http.MethodPut, fmt.Sprintf("/v1/uploads/%s/parts/%d", fileID, position), body)

		assertPartResponse(t, recorder, http.StatusCreated, int64(len(body)))
	})

	t.Run("replaces an existing part", func(t *testing.T) {
		app, mock := newTestApplication(t)
		body := []byte("replacement")
		expectBlobLookup(mock, true)
		expectPartStored(mock, body)

		recorder := serveRequest(t, app, http.MethodPut, fmt.Sprintf("/v1/uploads/%s/parts/%d", fileID, position), body)

		assertPartResponse(t, recorder, http.StatusOK, int64(len(body)))
	})

	t.Run("rejects an invalid file ID", func(t *testing.T) {
		app, _ := newTestApplication(t)

		recorder := serveRequest(t, app, http.MethodPut, "/v1/uploads/not-a-uuid/parts/0", []byte("part"))

		assertJSONResponse(t, recorder, http.StatusBadRequest)
	})

	t.Run("rejects a nonnumeric position", func(t *testing.T) {
		app, _ := newTestApplication(t)

		recorder := serveRequest(t, app, http.MethodPut, "/v1/uploads/"+fileID+"/parts/not-a-number", []byte("part"))

		assertJSONResponse(t, recorder, http.StatusBadRequest)
	})

	t.Run("rejects a negative position", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(getBlobQuery).
			WithArgs(data.FileID(fileID), -1).
			WillReturnRows(sqlmock.NewRows(blobColumns))

		recorder := serveRequest(t, app, http.MethodPut, "/v1/uploads/"+fileID+"/parts/-1", []byte("part"))

		assertJSONResponse(t, recorder, http.StatusBadRequest)
	})

	t.Run("stores one blob from a larger request body", func(t *testing.T) {
		previousMaxPartSize := blobs.BlobsMaxSize
		blobs.BlobsMaxSize = 4
		t.Cleanup(func() { blobs.BlobsMaxSize = previousMaxPartSize })

		app, mock, storagePath := newTestApplicationWithStoragePath(t)
		expectBlobLookup(mock, false)
		expectPartStored(mock, []byte("1234"))

		recorder := serveRequest(t, app, http.MethodPut, fmt.Sprintf("/v1/uploads/%s/parts/%d", fileID, position), []byte("12345"))

		assertPartResponse(t, recorder, http.StatusCreated, blobs.BlobsMaxSize)
		entries, err := os.ReadDir(storagePath)
		if err != nil {
			t.Fatalf("cannot read test blob directory: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("got %d stored files, expected one", len(entries))
		}
		stored, err := os.ReadFile(storagePath + "/" + entries[0].Name())
		if err != nil {
			t.Fatalf("cannot read stored blob: %v", err)
		}
		if !bytes.Equal(stored, []byte("1234")) {
			t.Errorf("got stored data %q, expected %q", stored, "1234")
		}
	})

	t.Run("returns not found for a missing or completed upload", func(t *testing.T) {
		app, mock := newTestApplication(t)
		expectBlobLookup(mock, false)
		mock.ExpectBegin()
		mock.ExpectQuery(lockUploadQuery).
			WithArgs(data.FileID(fileID), data.FileStatusUploading).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}))
		mock.ExpectRollback()

		recorder := serveRequest(t, app, http.MethodPut, fmt.Sprintf("/v1/uploads/%s/parts/%d", fileID, position), []byte("part"))

		assertJSONResponse(t, recorder, http.StatusNotFound)
		response := decodeResponse[struct {
			Error string `json:"error"`
		}](t, recorder)
		if response.Error != data.ErrFileNotFound.Error() {
			t.Errorf("got error %q, expected %q", response.Error, data.ErrFileNotFound.Error())
		}
	})

	t.Run("returns server error when storing metadata fails", func(t *testing.T) {
		app, mock := newTestApplication(t)
		expectBlobLookup(mock, false)
		mock.ExpectBegin().WillReturnError(errors.New("begin failed"))

		recorder := serveRequest(t, app, http.MethodPut, fmt.Sprintf("/v1/uploads/%s/parts/%d", fileID, position), []byte("part"))

		assertJSONResponse(t, recorder, http.StatusInternalServerError)
	})
}

func TestUploadsPartsDeleteEndpoint(t *testing.T) {
	const fileID = "32f11d17-b1f5-4fb0-94ca-614903665dff"
	const position = 2

	lockUploadQuery := regexp.QuoteMeta("SELECT TRUE FROM files WHERE id = $1 AND status = $2 FOR UPDATE")
	deletePartQuery := regexp.QuoteMeta("DELETE FROM file_blobs WHERE file_id = $1 AND position = $2")
	target := fmt.Sprintf("/v1/uploads/%s/parts/%d", fileID, position)

	expectUploadLock := func(mock sqlmock.Sqlmock) {
		mock.ExpectBegin()
		mock.ExpectQuery(lockUploadQuery).
			WithArgs(data.FileID(fileID), data.FileStatusUploading).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	}

	t.Run("removes an uploaded part", func(t *testing.T) {
		app, mock := newTestApplication(t)
		expectUploadLock(mock)
		mock.ExpectExec(deletePartQuery).
			WithArgs(data.FileID(fileID), position).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		recorder := serveRequest(t, app, http.MethodDelete, target, nil)

		if recorder.Code != http.StatusNoContent {
			t.Fatalf("got status %d, expected %d; body: %s", recorder.Code, http.StatusNoContent, recorder.Body.String())
		}
		if recorder.Body.Len() != 0 {
			t.Errorf("got response body %q, expected an empty body", recorder.Body.String())
		}
	})

	t.Run("rejects an invalid file ID", func(t *testing.T) {
		app, _ := newTestApplication(t)

		recorder := serveRequest(t, app, http.MethodDelete, "/v1/uploads/not-a-uuid/parts/0", nil)

		assertJSONResponse(t, recorder, http.StatusBadRequest)
	})

	t.Run("rejects a nonnumeric position", func(t *testing.T) {
		app, _ := newTestApplication(t)

		recorder := serveRequest(t, app, http.MethodDelete, "/v1/uploads/"+fileID+"/parts/not-a-number", nil)

		assertJSONResponse(t, recorder, http.StatusBadRequest)
	})

	t.Run("rejects a negative position", func(t *testing.T) {
		app, _ := newTestApplication(t)

		recorder := serveRequest(t, app, http.MethodDelete, "/v1/uploads/"+fileID+"/parts/-1", nil)

		assertJSONResponse(t, recorder, http.StatusBadRequest)
	})

	t.Run("returns not found for a missing or completed upload", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectBegin()
		mock.ExpectQuery(lockUploadQuery).
			WithArgs(data.FileID(fileID), data.FileStatusUploading).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}))
		mock.ExpectRollback()

		recorder := serveRequest(t, app, http.MethodDelete, target, nil)

		assertJSONResponse(t, recorder, http.StatusNotFound)
	})

	t.Run("returns not found for a missing part", func(t *testing.T) {
		app, mock := newTestApplication(t)
		expectUploadLock(mock)
		mock.ExpectExec(deletePartQuery).
			WithArgs(data.FileID(fileID), position).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()

		recorder := serveRequest(t, app, http.MethodDelete, target, nil)

		assertJSONResponse(t, recorder, http.StatusNotFound)
	})

	t.Run("returns server error on delete failure", func(t *testing.T) {
		app, mock := newTestApplication(t)
		expectUploadLock(mock)
		mock.ExpectExec(deletePartQuery).
			WithArgs(data.FileID(fileID), position).
			WillReturnError(errors.New("delete failed"))
		mock.ExpectRollback()

		recorder := serveRequest(t, app, http.MethodDelete, target, nil)

		assertJSONResponse(t, recorder, http.StatusInternalServerError)
	})
}

func TestUploadsCompletePostEndpoint(t *testing.T) {
	const (
		fileID  = "32f11d17-b1f5-4fb0-94ca-614903665dff"
		ownerID = "baab6197-3e2d-4969-8492-247bbba900dc"
	)

	lockUploadQuery := regexp.QuoteMeta("SELECT size FROM files WHERE id = $1 AND status = $2 FOR UPDATE")
	partsQuery := regexp.QuoteMeta(`
		SELECT fb.position, b.size
		FROM file_blobs fb
		JOIN blobs b ON b.hash = fb.blob_hash
		WHERE fb.file_id = $1
		ORDER BY fb.position`)
	updateQuery := regexp.QuoteMeta("UPDATE files SET status = $1 WHERE id = $2")
	getFileQuery := regexp.QuoteMeta("SELECT id, name, filetype, owner_id, status, size FROM files WHERE id = $1 AND status = $2")
	fileColumns := []string{"id", "name", "filetype", "owner_id", "status", "size"}
	target := "/v1/uploads/" + fileID + "/complete"
	declaredSize := blobs.BlobsMaxSize + 3

	expectCompletedFile := func(mock sqlmock.Sqlmock) {
		mock.ExpectQuery(getFileQuery).
			WithArgs(data.FileID(fileID), data.FileStatusReady).
			WillReturnRows(sqlmock.NewRows(fileColumns).
				AddRow(fileID, "movie.mov", "video/quicktime", ownerID, data.FileStatusReady, declaredSize))
	}

	assertCompletedFile := func(t *testing.T, recorder *httptest.ResponseRecorder) {
		t.Helper()
		assertJSONResponse(t, recorder, http.StatusOK)
		response := decodeResponse[struct {
			File data.File `json:"file"`
		}](t, recorder)
		if response.File.ID != data.FileID(fileID) || response.File.Status != data.FileStatusReady {
			t.Errorf("got file %+v, expected completed file %q", response.File, fileID)
		}
	}

	t.Run("validates and completes an upload", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectBegin()
		mock.ExpectQuery(lockUploadQuery).
			WithArgs(data.FileID(fileID), data.FileStatusUploading).
			WillReturnRows(sqlmock.NewRows([]string{"size"}).AddRow(declaredSize))
		mock.ExpectQuery(partsQuery).
			WithArgs(data.FileID(fileID)).
			WillReturnRows(sqlmock.NewRows([]string{"position", "size"}).
				AddRow(0, blobs.BlobsMaxSize).
				AddRow(1, int64(3)))
		mock.ExpectExec(updateQuery).
			WithArgs(data.FileStatusReady, data.FileID(fileID)).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		expectCompletedFile(mock)

		recorder := serveRequest(t, app, http.MethodPost, target, nil)

		assertCompletedFile(t, recorder)
	})

	t.Run("is idempotent for a completed upload", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectBegin()
		mock.ExpectQuery(lockUploadQuery).
			WithArgs(data.FileID(fileID), data.FileStatusUploading).
			WillReturnRows(sqlmock.NewRows([]string{"size"}))
		mock.ExpectRollback()
		expectCompletedFile(mock)

		recorder := serveRequest(t, app, http.MethodPost, target, nil)

		assertCompletedFile(t, recorder)
	})

	t.Run("rejects invalid part ordering", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectBegin()
		mock.ExpectQuery(lockUploadQuery).
			WithArgs(data.FileID(fileID), data.FileStatusUploading).
			WillReturnRows(sqlmock.NewRows([]string{"size"}).AddRow(declaredSize))
		mock.ExpectQuery(partsQuery).
			WithArgs(data.FileID(fileID)).
			WillReturnRows(sqlmock.NewRows([]string{"position", "size"}).
				AddRow(0, blobs.BlobsMaxSize).
				AddRow(2, int64(3)))
		mock.ExpectRollback()

		recorder := serveRequest(t, app, http.MethodPost, target, nil)

		assertJSONResponse(t, recorder, http.StatusConflict)
		response := decodeResponse[struct {
			Error string `json:"error"`
		}](t, recorder)
		if response.Error == "" {
			t.Error("expected completion validation details")
		}
	})

	t.Run("rejects an invalid file ID", func(t *testing.T) {
		app, _ := newTestApplication(t)

		recorder := serveRequest(t, app, http.MethodPost, "/v1/uploads/not-a-uuid/complete", nil)

		assertJSONResponse(t, recorder, http.StatusBadRequest)
	})

	t.Run("returns not found for a missing upload", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectBegin()
		mock.ExpectQuery(lockUploadQuery).
			WithArgs(data.FileID(fileID), data.FileStatusUploading).
			WillReturnRows(sqlmock.NewRows([]string{"size"}))
		mock.ExpectRollback()
		mock.ExpectQuery(getFileQuery).
			WithArgs(data.FileID(fileID), data.FileStatusReady).
			WillReturnRows(sqlmock.NewRows(fileColumns))

		recorder := serveRequest(t, app, http.MethodPost, target, nil)

		assertJSONResponse(t, recorder, http.StatusNotFound)
	})

	t.Run("returns server error on storage failure", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectBegin().WillReturnError(errors.New("begin failed"))

		recorder := serveRequest(t, app, http.MethodPost, target, nil)

		assertJSONResponse(t, recorder, http.StatusInternalServerError)
	})
}

func TestUploadsDeleteEndpoint(t *testing.T) {
	const fileID = "32f11d17-b1f5-4fb0-94ca-614903665dff"
	deleteQuery := regexp.QuoteMeta("DELETE FROM files WHERE id = $1 AND status = $2")
	target := "/v1/uploads/" + fileID

	t.Run("cancels an unfinished upload", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectExec(deleteQuery).
			WithArgs(data.FileID(fileID), data.FileStatusUploading).
			WillReturnResult(sqlmock.NewResult(0, 1))

		recorder := serveRequest(t, app, http.MethodDelete, target, nil)

		if recorder.Code != http.StatusNoContent {
			t.Fatalf("got status %d, expected %d; body: %s", recorder.Code, http.StatusNoContent, recorder.Body.String())
		}
		if recorder.Body.Len() != 0 {
			t.Errorf("got response body %q, expected an empty body", recorder.Body.String())
		}
	})

	t.Run("rejects an invalid file ID", func(t *testing.T) {
		app, _ := newTestApplication(t)

		recorder := serveRequest(t, app, http.MethodDelete, "/v1/uploads/not-a-uuid", nil)

		assertJSONResponse(t, recorder, http.StatusBadRequest)
	})

	t.Run("returns not found for a missing or completed upload", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectExec(deleteQuery).
			WithArgs(data.FileID(fileID), data.FileStatusUploading).
			WillReturnResult(sqlmock.NewResult(0, 0))

		recorder := serveRequest(t, app, http.MethodDelete, target, nil)

		assertJSONResponse(t, recorder, http.StatusNotFound)
	})

	t.Run("returns server error on delete failure", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectExec(deleteQuery).
			WithArgs(data.FileID(fileID), data.FileStatusUploading).
			WillReturnError(errors.New("delete failed"))

		recorder := serveRequest(t, app, http.MethodDelete, target, nil)

		assertJSONResponse(t, recorder, http.StatusInternalServerError)
	})
}

func TestFilesContentGetEndpoint(t *testing.T) {
	const (
		fileID  = "32f11d17-b1f5-4fb0-94ca-614903665dff"
		ownerID = "baab6197-3e2d-4969-8492-247bbba900dc"
	)
	selectFileQuery := regexp.QuoteMeta("SELECT id, name, filetype, owner_id, status, size FROM files WHERE id = $1 AND status = $2")
	selectBlobsQuery := regexp.QuoteMeta(`SELECT b.hash, b.size
		FROM files f
		LEFT JOIN file_blobs fb ON fb.file_id = f.id
		LEFT JOIN blobs b ON b.hash = fb.blob_hash
		WHERE f.id = $1
		ORDER BY fb.position`)

	expectReadyFile := func(mock sqlmock.Sqlmock) {
		mock.ExpectQuery(selectFileQuery).
			WithArgs(data.FileID(fileID), data.FileStatusReady).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "filetype", "owner_id", "status", "size"}).
				AddRow(fileID, "file.bin", "application/octet-stream", ownerID, data.FileStatusReady, int64(11)))
	}

	expectReadableFile := func(mock sqlmock.Sqlmock) {
		expectReadyFile(mock)
		expectReadyFile(mock)
	}

	t.Run("streams blobs in order", func(t *testing.T) {
		app, mock, storagePath := newTestApplicationWithStoragePath(t)
		expectReadableFile(mock)
		mock.ExpectQuery(selectBlobsQuery).
			WithArgs(data.FileID(fileID)).
			WillReturnRows(sqlmock.NewRows([]string{"hash", "size"}).
				AddRow("first-blob", int64(6)).
				AddRow("second-blob", int64(5)))
		if err := os.WriteFile(storagePath+"/first-blob", []byte("hello "), 0o644); err != nil {
			t.Fatalf("write first blob: %v", err)
		}
		if err := os.WriteFile(storagePath+"/second-blob", []byte("world"), 0o644); err != nil {
			t.Fatalf("write second blob: %v", err)
		}

		recorder := serveRequest(t, app, http.MethodGet, "/v1/files/"+fileID+"/content", nil)

		if recorder.Code != http.StatusOK {
			t.Fatalf("got status %d, expected %d", recorder.Code, http.StatusOK)
		}
		if got := recorder.Header().Get("Content-Type"); got != "application/octet-stream" {
			t.Errorf("got Content-Type %q, expected application/octet-stream", got)
		}
		if got := recorder.Header().Get("Content-Length"); got != "11" {
			t.Errorf("got Content-Length %q, expected 11", got)
		}
		if got := recorder.Body.String(); got != "hello world" {
			t.Errorf("got body %q, expected %q", got, "hello world")
		}
	})

	t.Run("returns an empty ready file", func(t *testing.T) {
		app, mock := newTestApplication(t)
		expectReadableFile(mock)
		mock.ExpectQuery(selectBlobsQuery).
			WithArgs(data.FileID(fileID)).
			WillReturnRows(sqlmock.NewRows([]string{"hash", "size"}).AddRow(nil, nil))

		recorder := serveRequest(t, app, http.MethodGet, "/v1/files/"+fileID+"/content", nil)

		if recorder.Code != http.StatusOK {
			t.Fatalf("got status %d, expected %d", recorder.Code, http.StatusOK)
		}
		if recorder.Body.Len() != 0 {
			t.Errorf("got body %q, expected an empty body", recorder.Body.String())
		}
	})

	t.Run("returns not found before streaming", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(selectFileQuery).
			WithArgs(data.FileID(fileID), data.FileStatusReady).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "filetype", "owner_id", "status", "size"}))

		recorder := serveRequest(t, app, http.MethodGet, "/v1/files/"+fileID+"/content", nil)

		assertJSONResponse(t, recorder, http.StatusNotFound)
	})

	t.Run("returns server error before streaming", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(selectFileQuery).
			WithArgs(data.FileID(fileID), data.FileStatusReady).
			WillReturnError(errors.New("select failed"))

		recorder := serveRequest(t, app, http.MethodGet, "/v1/files/"+fileID+"/content", nil)

		assertJSONResponse(t, recorder, http.StatusInternalServerError)
	})

	t.Run("does not write a second response after streaming starts", func(t *testing.T) {
		app, mock, storagePath := newTestApplicationWithStoragePath(t)
		var logs bytes.Buffer
		app.logger = log.New(&logs, "", 0)
		expectReadableFile(mock)
		mock.ExpectQuery(selectBlobsQuery).
			WithArgs(data.FileID(fileID)).
			WillReturnRows(sqlmock.NewRows([]string{"hash", "size"}).
				AddRow("first-blob", int64(6)).
				AddRow("missing-blob", int64(5)))
		if err := os.WriteFile(storagePath+"/first-blob", []byte("hello "), 0o644); err != nil {
			t.Fatalf("write first blob: %v", err)
		}

		recorder := serveRequest(t, app, http.MethodGet, "/v1/files/"+fileID+"/content", nil)

		if recorder.Code != http.StatusOK {
			t.Fatalf("got status %d, expected committed status %d", recorder.Code, http.StatusOK)
		}
		if got := recorder.Body.String(); got != "hello " {
			t.Errorf("got body %q, expected only the successfully streamed prefix", got)
		}
		if logs.Len() == 0 {
			t.Error("expected the mid-stream read failure to be logged")
		}
	})

	t.Run("stops quietly when request is canceled", func(t *testing.T) {
		app, _ := newTestApplication(t)
		req := httptest.NewRequest(http.MethodGet, "/v1/files/"+fileID+"/content", nil)
		ctx, cancel := context.WithCancel(req.Context())
		cancel()
		req = req.WithContext(ctx)
		recorder := httptest.NewRecorder()

		app.routes().ServeHTTP(recorder, req)

		if recorder.Body.Len() != 0 {
			t.Errorf("got body %q for canceled request, expected none", recorder.Body.String())
		}
	})
}

func TestFilesContentHeadEndpoint(t *testing.T) {
	const (
		fileID  = "32f11d17-b1f5-4fb0-94ca-614903665dff"
		ownerID = "baab6197-3e2d-4969-8492-247bbba900dc"
	)
	selectFileQuery := regexp.QuoteMeta("SELECT id, name, filetype, owner_id, status, size FROM files WHERE id = $1 AND status = $2")
	fileColumns := []string{"id", "name", "filetype", "owner_id", "status", "size"}

	t.Run("returns download metadata without reading blobs", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(selectFileQuery).
			WithArgs(data.FileID(fileID), data.FileStatusReady).
			WillReturnRows(sqlmock.NewRows(fileColumns).
				AddRow(fileID, "movie.mov", "video/quicktime", ownerID, data.FileStatusReady, int64(104857600)))

		recorder := serveRequest(t, app, http.MethodHead, "/v1/files/"+fileID+"/content", nil)

		if recorder.Code != http.StatusOK {
			t.Fatalf("got status %d, expected %d", recorder.Code, http.StatusOK)
		}
		if got := recorder.Header().Get("Content-Type"); got != "video/quicktime" {
			t.Errorf("got Content-Type %q, expected video/quicktime", got)
		}
		if got := recorder.Header().Get("Content-Length"); got != "104857600" {
			t.Errorf("got Content-Length %q, expected 104857600", got)
		}
		if recorder.Body.Len() != 0 {
			t.Errorf("got body %q, expected an empty body", recorder.Body.String())
		}
	})

	t.Run("returns metadata for an empty ready file", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(selectFileQuery).
			WithArgs(data.FileID(fileID), data.FileStatusReady).
			WillReturnRows(sqlmock.NewRows(fileColumns).
				AddRow(fileID, "empty.bin", "application/octet-stream", ownerID, data.FileStatusReady, int64(0)))

		recorder := serveRequest(t, app, http.MethodHead, "/v1/files/"+fileID+"/content", nil)

		if recorder.Code != http.StatusOK {
			t.Fatalf("got status %d, expected %d", recorder.Code, http.StatusOK)
		}
		if got := recorder.Header().Get("Content-Length"); got != "0" {
			t.Errorf("got Content-Length %q, expected 0", got)
		}
		if recorder.Body.Len() != 0 {
			t.Errorf("got body %q, expected an empty body", recorder.Body.String())
		}
	})

	t.Run("returns not found without a body", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(selectFileQuery).
			WithArgs(data.FileID(fileID), data.FileStatusReady).
			WillReturnRows(sqlmock.NewRows(fileColumns))

		recorder := serveRequest(t, app, http.MethodHead, "/v1/files/"+fileID+"/content", nil)

		if recorder.Code != http.StatusNotFound {
			t.Fatalf("got status %d, expected %d", recorder.Code, http.StatusNotFound)
		}
		if recorder.Body.Len() != 0 {
			t.Errorf("got body %q, expected an empty body", recorder.Body.String())
		}
	})

	t.Run("returns server error without a body", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(selectFileQuery).
			WithArgs(data.FileID(fileID), data.FileStatusReady).
			WillReturnError(errors.New("select failed"))

		recorder := serveRequest(t, app, http.MethodHead, "/v1/files/"+fileID+"/content", nil)

		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("got status %d, expected %d", recorder.Code, http.StatusInternalServerError)
		}
		if recorder.Body.Len() != 0 {
			t.Errorf("got body %q, expected an empty body", recorder.Body.String())
		}
	})

	t.Run("returns bad request without a body when id is missing", func(t *testing.T) {
		app, _ := newTestApplication(t)
		req := httptest.NewRequest(http.MethodHead, "/v1/files//content", nil)
		recorder := httptest.NewRecorder()

		app.filesContentHeadHandler(recorder, req)

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("got status %d, expected %d", recorder.Code, http.StatusBadRequest)
		}
		if recorder.Body.Len() != 0 {
			t.Errorf("got body %q, expected an empty body", recorder.Body.String())
		}
	})

	t.Run("stops quietly when request is canceled", func(t *testing.T) {
		app, _ := newTestApplication(t)
		req := httptest.NewRequest(http.MethodHead, "/v1/files/"+fileID+"/content", nil)
		ctx, cancel := context.WithCancel(req.Context())
		cancel()
		req = req.WithContext(ctx)
		recorder := httptest.NewRecorder()

		app.routes().ServeHTTP(recorder, req)

		if recorder.Body.Len() != 0 {
			t.Errorf("got body %q for canceled request, expected none", recorder.Body.String())
		}
	})
}
