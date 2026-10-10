package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"gopherdisk/internal/data"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func newTestApplication(t *testing.T) (*Application, sqlmock.Sqlmock) {
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

	storage, err := data.MakeStorage(t.TempDir(), db)
	if err != nil {
		t.Fatalf("cannot create test storage: %v", err)
	}

	return &Application{
		storage: storage,
		logger:  log.New(io.Discard, "", 0),
	}, mock
}

func serveRequest(t *testing.T, app *Application, method, target string, body []byte) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	app.routes().ServeHTTP(recorder, req)

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

func TestFilesUploadPostEndpoint(t *testing.T) {
	const (
		fileID  = "32f11d17-b1f5-4fb0-94ca-614903665dff"
		ownerID = "baab6197-3e2d-4969-8492-247bbba900dc"
	)
	query := regexp.QuoteMeta(`
		INSERT INTO files (id, name, owner_id, filetype, size, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`)
	body := []byte(`{"owner_id":"` + ownerID + `","name":"movie.mov","file_type":"MOV","size":1024}`)

	t.Run("initializes upload", func(t *testing.T) {
		app, mock := newTestApplication(t)
		mock.ExpectQuery(query).
			WithArgs(sqlmock.AnyArg(), "movie.mov", data.UserID(ownerID), data.FileKind("MOV"), int64(1024), data.FileStatusUploading).
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
