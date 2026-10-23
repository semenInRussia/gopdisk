package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"gopherdisk/internal/data"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestEnableCORS(t *testing.T) {
	app := &Application{}
	app.cfg.cors.trustedOrigins = []string{"https://app.example.com", "http://localhost:5173"}

	tests := []struct {
		name          string
		origin        string
		allowedOrigin string
	}{
		{name: "no origin"},
		{name: "trusted origin", origin: "https://app.example.com", allowedOrigin: "https://app.example.com"},
		{name: "second trusted origin", origin: "http://localhost:5173", allowedOrigin: "http://localhost:5173"},
		{name: "untrusted origin", origin: "https://evil.example.com"},
		{name: "different scheme", origin: "http://app.example.com"},
		{name: "different port", origin: "https://app.example.com:8443"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.origin != "" {
				request.Header.Set("Origin", tt.origin)
			}
			recorder := httptest.NewRecorder()

			app.enableCORS(next).ServeHTTP(recorder, request)

			if !called {
				t.Error("next handler was not called")
			}
			if recorder.Code != http.StatusNoContent {
				t.Errorf("got status %d, want %d", recorder.Code, http.StatusNoContent)
			}
			if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != tt.allowedOrigin {
				t.Errorf("got Access-Control-Allow-Origin %q, want %q", got, tt.allowedOrigin)
			}
			assertHeaderValues(t, recorder, "Vary", []string{"Origin", "Access-Control-Request-Method"})
		})
	}
}

func TestEnableCORSPreflight(t *testing.T) {
	app := &Application{}
	app.cfg.cors.trustedOrigins = []string{"https://app.example.com"}

	for _, tt := range []struct {
		name       string
		origin     string
		preflight  bool
		wantStatus int
		wantCalled bool
	}{
		{name: "trusted preflight", origin: "https://app.example.com", preflight: true, wantStatus: http.StatusOK},
		{name: "trusted OPTIONS without requested method", origin: "https://app.example.com", wantStatus: http.StatusNoContent, wantCalled: true},
		{name: "untrusted preflight", origin: "https://evil.example.com", preflight: true, wantStatus: http.StatusNoContent, wantCalled: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodOptions, "/v1/files/file-id", nil)
			request.Header.Set("Origin", tt.origin)
			if tt.preflight {
				request.Header.Set("Access-Control-Request-Method", http.MethodPut)
			}
			recorder := httptest.NewRecorder()

			app.enableCORS(next).ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", recorder.Code, tt.wantStatus)
			}
			if called != tt.wantCalled {
				t.Errorf("next handler called = %v, want %v", called, tt.wantCalled)
			}

			if tt.origin == "https://app.example.com" {
				if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != tt.origin {
					t.Errorf("got Access-Control-Allow-Origin %q, want %q", got, tt.origin)
				}
			} else if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
				t.Errorf("got unexpected Access-Control-Allow-Origin %q", got)
			}

			if tt.preflight && tt.origin == "https://app.example.com" {
				if got := recorder.Header().Get("Access-Control-Allow-Methods"); got != "OPTIONS, PUT, PATCH, DELETE" {
					t.Errorf("got Access-Control-Allow-Methods %q", got)
				}
				if got := recorder.Header().Get("Access-Control-Allow-Headers"); got != "Authorization, Content-Type" {
					t.Errorf("got Access-Control-Allow-Headers %q", got)
				}
			} else {
				if got := recorder.Header().Get("Access-Control-Allow-Methods"); got != "" {
					t.Errorf("got unexpected Access-Control-Allow-Methods %q", got)
				}
				if got := recorder.Header().Get("Access-Control-Allow-Headers"); got != "" {
					t.Errorf("got unexpected Access-Control-Allow-Headers %q", got)
				}
			}
		})
	}
}

func TestRoutesCORSHeadersIncludeAuthorizationVary(t *testing.T) {
	app, _ := newTestApplication(t)
	app.cfg.cors.trustedOrigins = []string{"https://app.example.com"}
	request := httptest.NewRequest(http.MethodGet, "/v1/healthcheck", nil)
	request.Header.Set("Origin", "https://app.example.com")
	recorder := httptest.NewRecorder()

	app.routes().ServeHTTP(recorder, request)

	assertHeaderValues(t, recorder, "Vary", []string{"Origin", "Access-Control-Request-Method", "Authorization"})
}

func assertHeaderValues(t *testing.T, recorder *httptest.ResponseRecorder, name string, want []string) {
	t.Helper()

	got := recorder.Header().Values(name)
	if len(got) != len(want) {
		t.Fatalf("got %s values %q, want %q", name, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %s values %q, want %q", name, got, want)
			return
		}
	}
}

const testAuthenticationToken = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

const filePermissionQuery = `SELECT f.owner_id = $2 OR EXISTS (
	SELECT 1 FROM file_permissions p
	WHERE p.file_id = f.id AND p.user_id = $2 AND p.code = $3
	) FROM files f WHERE f.id = $1`

func expectFilePermission(mock sqlmock.Sqlmock, fileID, code string, allowed bool) {
	mock.ExpectQuery(regexp.QuoteMeta(filePermissionQuery)).
		WithArgs(data.FileID(fileID), data.UserID("baab6197-3e2d-4969-8492-247bbba900dc"), code).
		WillReturnRows(sqlmock.NewRows([]string{"allowed"}).AddRow(allowed))
}

func newFileAuthorizedTestApplication(t *testing.T, fileID, code string) (*Application, sqlmock.Sqlmock) {
	t.Helper()
	app, mock, _ := newFileAuthorizedTestApplicationWithStoragePath(t, fileID, code)
	return app, mock
}

func newFileAuthorizedTestApplicationWithStoragePath(t *testing.T, fileID, code string) (*Application, sqlmock.Sqlmock, string) {
	t.Helper()
	app, mock, path := newAuthenticatedTestApplicationWithStoragePath(t)
	expectFilePermission(mock, fileID, code, true)
	return app, mock, path
}

func expectAuthentication(mock sqlmock.Sqlmock, activated bool) {
	hash := sha256.Sum256([]byte(testAuthenticationToken))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT u.id, u.name, u.email, u.password_hash, u.activated
		FROM users u
		INNER JOIN tokens t ON t.user_id = u.id
		WHERE t.hash = $1 AND t.scope = $2 AND t.expiry > now()`)).
		WithArgs(hash[:], data.ScopeAuthentication).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "email", "password_hash", "activated"}).
			AddRow("baab6197-3e2d-4969-8492-247bbba900dc", "Alice", "alice@example.com", []byte("!"), activated))
}

func newAuthenticatedTestApplication(t *testing.T) (*Application, sqlmock.Sqlmock) {
	t.Helper()
	app, mock, _ := newAuthenticatedTestApplicationWithStoragePath(t)
	return app, mock
}

func newAuthenticatedTestApplicationWithStoragePath(t *testing.T) (*Application, sqlmock.Sqlmock, string) {
	t.Helper()
	app, mock, path := newTestApplicationWithStoragePath(t)
	expectAuthentication(mock, true)
	return app, mock, path
}

func TestRequireUser(t *testing.T) {
	app := &Application{}
	for _, tt := range []struct {
		name                string
		user                *data.User
		authenticatedStatus int
		activatedStatus     int
	}{
		{"anonymous", data.AnonymousUser, http.StatusUnauthorized, http.StatusUnauthorized},
		{"inactive", &data.User{ID: "user-id"}, http.StatusNoContent, http.StatusForbidden},
		{"active", &data.User{ID: "user-id", Activated: true}, http.StatusNoContent, http.StatusNoContent},
	} {
		for _, guard := range []struct {
			name   string
			wrap   func(http.HandlerFunc) http.HandlerFunc
			status int
		}{
			{"authenticated", app.requireAuthenticatedUser, tt.authenticatedStatus},
			{"activated", app.requireActivatedUser, tt.activatedStatus},
		} {
			t.Run(guard.name+"/"+tt.name, func(t *testing.T) {
				called := false
				next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					called = true
					if app.contextGetUser(r) != tt.user {
						t.Error("user context changed")
					}
					w.WriteHeader(http.StatusNoContent)
				})
				r := app.contextSetUser(httptest.NewRequest(http.MethodGet, "/", nil), tt.user)
				w := httptest.NewRecorder()
				guard.wrap(next).ServeHTTP(w, r)
				if w.Code != guard.status || called != (guard.status == http.StatusNoContent) {
					t.Fatalf("got status %d, next called %v; want status %d", w.Code, called, guard.status)
				}
				if guard.status != http.StatusNoContent {
					assertJSONResponse(t, w, guard.status)
				}
			})
		}
	}
}

func TestProtectedRoutes(t *testing.T) {
	const id = "32f11d17-b1f5-4fb0-94ca-614903665dff"
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/v1/users/" + id},
		{http.MethodDelete, "/v1/users/" + id},
		{http.MethodGet, "/v1/users/" + id + "/files"},
		{http.MethodGet, "/v1/users/" + id + "/uploads"},
		{http.MethodGet, "/v1/files/" + id},
		{http.MethodDelete, "/v1/files/" + id},
		{http.MethodPatch, "/v1/files/" + id},
		{http.MethodGet, "/v1/files/" + id + "/content"},
		{http.MethodHead, "/v1/files/" + id + "/content"},
		{http.MethodPost, "/v1/files/upload"},
		{http.MethodGet, "/v1/uploads/" + id},
		{http.MethodPut, "/v1/uploads/" + id + "/parts/0"},
		{http.MethodDelete, "/v1/uploads/" + id + "/parts/0"},
		{http.MethodPost, "/v1/uploads/" + id + "/complete"},
		{http.MethodDelete, "/v1/uploads/" + id},
		{http.MethodGet, "/v1/files/" + id + "/permissions"},
		{http.MethodPut, "/v1/files/" + id + "/permissions/" + id},
		{http.MethodDelete, "/v1/files/" + id + "/permissions/" + id},
	} {
		for _, authenticated := range []bool{false, true} {
			t.Run(route.method+" "+route.path+fmt.Sprint(" authenticated=", authenticated), func(t *testing.T) {
				app, mock := newTestApplication(t)
				r := httptest.NewRequest(route.method, route.path, nil)
				status := http.StatusUnauthorized
				if authenticated {
					expectAuthentication(mock, false)
					r.Header.Set("Authorization", "Bearer "+testAuthenticationToken)
					status = http.StatusForbidden
				}
				w := httptest.NewRecorder()
				app.routes().ServeHTTP(w, r)
				assertJSONResponse(t, w, status)
			})
		}
	}
}

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
