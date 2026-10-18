package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"gopherdisk/internal/data"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/julienschmidt/httprouter"
)

func TestFileRoutesRequirePermission(t *testing.T) {
	for _, route := range []struct{ method, path, code string }{
		{http.MethodGet, "/v1/files/" + permissionFileID, data.PermissionRead},
		{http.MethodHead, "/v1/files/" + permissionFileID + "/content", data.PermissionRead},
		{http.MethodGet, "/v1/files/" + permissionFileID + "/content", data.PermissionRead},
		{http.MethodPatch, "/v1/files/" + permissionFileID, data.PermissionWrite},
		{http.MethodDelete, "/v1/files/" + permissionFileID, data.PermissionDelete},
		{http.MethodGet, "/v1/uploads/" + permissionFileID, data.PermissionRead},
		{http.MethodPut, "/v1/uploads/" + permissionFileID + "/parts/0", data.PermissionWrite},
		{http.MethodDelete, "/v1/uploads/" + permissionFileID + "/parts/0", data.PermissionWrite},
		{http.MethodPost, "/v1/uploads/" + permissionFileID + "/complete", data.PermissionWrite},
		{http.MethodDelete, "/v1/uploads/" + permissionFileID, data.PermissionDelete},
	} {
		for _, scenario := range []string{"denied", "missing file", "database error"} {
			t.Run(route.method+" "+route.path+"/"+scenario, func(t *testing.T) {
				app, mock := newAuthenticatedTestApplication(t)
				expectation := mock.ExpectQuery(regexp.QuoteMeta(filePermissionQuery)).
					WithArgs(data.FileID(permissionFileID), data.UserID(permissionOwnerID), route.code)
				status := http.StatusForbidden
				switch scenario {
				case "denied":
					expectation.WillReturnRows(sqlmock.NewRows([]string{"allowed"}).AddRow(false))
				case "missing file":
					expectation.WillReturnRows(sqlmock.NewRows([]string{"allowed"}))
					status = http.StatusNotFound
				case "database error":
					expectation.WillReturnError(errors.New("lookup failed"))
					status = http.StatusInternalServerError
				}
				w := serveRequest(t, app, route.method, route.path, nil)
				assertJSONResponse(t, w, status)
			})
		}
	}
}

func TestRequireFilePermission(t *testing.T) {
	for _, param := range []string{"id", "file_id"} {
		for _, code := range []string{data.PermissionRead, data.PermissionWrite, data.PermissionDelete} {
			t.Run(param+"/"+code, func(t *testing.T) {
				app, mock := newTestApplication(t)
				expectFilePermission(mock, permissionFileID, code, true)
				r := httptest.NewRequest(http.MethodGet, "/", nil)
				r = r.WithContext(context.WithValue(r.Context(), httprouter.ParamsKey, httprouter.Params{{Key: param, Value: permissionFileID}}))
				r = app.contextSetUser(r, &data.User{ID: data.UserID(permissionOwnerID), Activated: true})
				w := httptest.NewRecorder()
				app.requireFilePermission(param, code, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNoContent)
				}).ServeHTTP(w, r)
				if w.Code != http.StatusNoContent {
					t.Fatalf("got %d: %s", w.Code, w.Body.String())
				}
			})
		}
	}
}

func TestUserRoutesRequireCurrentUser(t *testing.T) {
	for _, route := range []struct{ method, suffix string }{
		{http.MethodGet, ""}, {http.MethodDelete, ""},
		{http.MethodGet, "/files"}, {http.MethodGet, "/uploads"},
	} {
		for _, id := range []string{permissionUserID, "invalid"} {
			t.Run(route.method+route.suffix+"/"+id, func(t *testing.T) {
				app, _ := newAuthenticatedTestApplication(t)
				w := serveRequest(t, app, route.method, "/v1/users/"+id+route.suffix, nil)
				status := http.StatusForbidden
				if id == "invalid" {
					status = http.StatusBadRequest
				}
				assertJSONResponse(t, w, status)
			})
		}
	}
}

func TestRequireCurrentUserAcceptsEquivalentUUID(t *testing.T) {
	app := &Application{}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(context.WithValue(r.Context(), httprouter.ParamsKey, httprouter.Params{{Key: "id", Value: strings.ToUpper(permissionOwnerID)}}))
	r = app.contextSetUser(r, &data.User{ID: data.UserID(permissionOwnerID), Activated: true})
	w := httptest.NewRecorder()
	app.requireCurrentUser(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}).ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
}

func TestUploadCannotUseAnotherOwner(t *testing.T) {
	app, _ := newAuthenticatedTestApplication(t)
	body := []byte(`{"owner_id":"` + permissionUserID + `","name":"test","type":"TXT","size":0}`)
	w := serveRequest(t, app, http.MethodPost, "/v1/files/upload", body)
	assertJSONResponse(t, w, http.StatusForbidden)
}

func TestPermissionsRemainOwnerOnly(t *testing.T) {
	// Even a user with file access cannot inspect or grant permissions.
	// These routes must use ownership, without consulting file permission rows.
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			app, mock := newAuthenticatedTestApplication(t)
			expectPermissionsFile(mock, permissionUserID)
			path := "/v1/files/" + permissionFileID + "/permissions"
			if method != http.MethodGet {
				path += "/" + permissionOwnerID
			}
			w := serveRequest(t, app, method, path, []byte(`{"codes":["files:write"]}`))
			assertJSONResponse(t, w, http.StatusForbidden)
		})
	}
}
