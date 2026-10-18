package main

import (
	"crypto/sha256"
	"errors"
	"gopherdisk/internal/data"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAuthenticate(t *testing.T) {
	const token = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const userID = "baab6197-3e2d-4969-8492-247bbba900dc"
	query := regexp.QuoteMeta(`SELECT u.id, u.name, u.email, u.password_hash, u.activated
		FROM users u
		INNER JOIN tokens t ON t.user_id = u.id
		WHERE t.hash = $1 AND t.scope = $2 AND t.expiry > now()`)
	columns := []string{"id", "name", "email", "password_hash", "activated"}

	for _, tt := range []struct {
		name   string
		header string
		lookup bool
		err    error
		absent bool
		status int
	}{
		{name: "anonymous", status: http.StatusNoContent},
		{name: "valid token", header: "Bearer " + token, lookup: true, status: http.StatusNoContent},
		{name: "wrong scheme", header: "Basic " + token, status: http.StatusUnauthorized},
		{name: "missing token", header: "Bearer", status: http.StatusUnauthorized},
		{name: "empty token", header: "Bearer ", status: http.StatusUnauthorized},
		{name: "short token", header: "Bearer short", status: http.StatusUnauthorized},
		{name: "long token", header: "Bearer " + token + "A", status: http.StatusUnauthorized},
		{name: "extra parts", header: "Bearer " + token + " extra", status: http.StatusUnauthorized},
		{name: "extra space", header: "Bearer  " + token, status: http.StatusUnauthorized},
		{name: "invalid or expired token", header: "Bearer " + token, lookup: true, absent: true, status: http.StatusUnauthorized},
		{name: "database failure", header: "Bearer " + token, lookup: true, err: errors.New("database unavailable"), status: http.StatusInternalServerError},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, mock := newTestApplication(t)
			if tt.lookup {
				hash := sha256.Sum256([]byte(token))
				expectation := mock.ExpectQuery(query).WithArgs(hash[:], data.ScopeAuthentication)
				switch {
				case tt.err != nil:
					expectation.WillReturnError(tt.err)
				case tt.absent:
					expectation.WillReturnRows(sqlmock.NewRows(columns))
				default:
					expectation.WillReturnRows(sqlmock.NewRows(columns).AddRow(userID, "Alice", "alice@example.com", []byte("!"), true))
				}
			}

			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				user := app.contextGetUser(r)
				if tt.header == "" {
					if !user.IsAnonymous() {
						t.Error("expected anonymous user")
					}
				} else if user.ID != data.UserID(userID) || user.Name != "Alice" || user.Email != "alice@example.com" || !user.Activated {
					t.Errorf("unexpected authenticated user: %+v", user)
				}
				w.WriteHeader(http.StatusNoContent)
			})
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				r.Header.Set("Authorization", tt.header)
			}
			w := httptest.NewRecorder()
			app.authenticate(next).ServeHTTP(w, r)

			if w.Code != tt.status {
				t.Fatalf("got status %d, want %d; body: %s", w.Code, tt.status, w.Body.String())
			}
			if called != (tt.status == http.StatusNoContent) {
				t.Errorf("next handler called: %v", called)
			}
			if got := w.Header().Get("Vary"); got != "Authorization" {
				t.Errorf("got Vary %q", got)
			}
			if tt.status == http.StatusUnauthorized {
				assertJSONResponse(t, w, tt.status)
				if got := w.Header().Get("WWW-Authenticate"); got != "Bearer" {
					t.Errorf("got WWW-Authenticate %q", got)
				}
				response := decodeResponse[struct {
					Error string `json:"error"`
				}](t, w)
				if response.Error != "invalid or missing authentication token" {
					t.Errorf("unexpected error: %q", response.Error)
				}
			}
		})
	}
}

func TestRoutesAuthenticate(t *testing.T) {
	app, _ := newTestApplication(t)
	r := httptest.NewRequest(http.MethodGet, "/v1/healthcheck", nil)
	r.Header.Set("Authorization", "Bearer invalid")
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, r)
	assertJSONResponse(t, w, http.StatusUnauthorized)
}
