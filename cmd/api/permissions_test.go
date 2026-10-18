package main

import (
	"errors"
	"net/http"
	"regexp"
	"testing"

	"gopherdisk/internal/data"

	"github.com/DATA-DOG/go-sqlmock"
)

const (
	permissionFileID  = "32f11d17-b1f5-4fb0-94ca-614903665dff"
	permissionOwnerID = "baab6197-3e2d-4969-8492-247bbba900dc"
	permissionUserID  = "593fc45d-8640-4e4a-a488-aa753b27895c"
)

func expectPermissionsFile(mock sqlmock.Sqlmock, owner string) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name, filetype, owner_id, status, size FROM files WHERE id = $1 AND status = $2")).
		WithArgs(data.FileID(permissionFileID), data.FileStatusReady).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "filetype", "owner_id", "status", "size"}).
			AddRow(permissionFileID, "test.txt", "TXT", owner, data.FileStatusReady, 0))
}

func expectPermissionsReplacement(mock sqlmock.Sqlmock, codes []string) {
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM files WHERE id = $1 FOR UPDATE")).
		WithArgs(data.FileID(permissionFileID)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(permissionFileID))
	mock.ExpectExec("DELETE FROM file_permissions").WithArgs(data.FileID(permissionFileID), data.UserID(permissionUserID)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	for _, code := range codes {
		mock.ExpectExec("INSERT INTO file_permissions").WithArgs(data.FileID(permissionFileID), data.UserID(permissionUserID), code).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()
}

func TestPermissionsGetEndpoint(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "permissions", true: "empty"}[empty], func(t *testing.T) {
			app, mock := newAuthenticatedTestApplication(t)
			expectPermissionsFile(mock, permissionOwnerID)
			rows := sqlmock.NewRows([]string{"file_id", "user_id", "code"})
			if !empty {
				rows.AddRow(permissionFileID, permissionUserID, data.PermissionRead)
			}
			mock.ExpectQuery("SELECT file_id, user_id, code FROM file_permissions").WithArgs(data.FileID(permissionFileID)).WillReturnRows(rows)
			w := serveRequest(t, app, http.MethodGet, "/v1/files/"+permissionFileID+"/permissions", nil)
			assertJSONResponse(t, w, http.StatusOK)
			response := decodeResponse[struct {
				Permissions []data.FilePermission `json:"permissions"`
			}](t, w)
			if response.Permissions == nil {
				t.Fatal("expected JSON array, not null")
			}
			if empty && len(response.Permissions) != 0 {
				t.Fatalf("expected empty permissions: %v", response.Permissions)
			}
			if !empty && (len(response.Permissions) != 1 || response.Permissions[0] != (data.FilePermission{FileID: data.FileID(permissionFileID), UserID: data.UserID(permissionUserID), Code: data.PermissionRead})) {
				t.Fatalf("unexpected permissions: %v", response.Permissions)
			}
		})
	}
}

func TestPermissionsPutEndpoint(t *testing.T) {
	for _, tt := range []struct {
		name   string
		body   string
		codes  []string
		status int
	}{
		{"replace", `{"codes":["files:read","files:write"]}`, []string{data.PermissionRead, data.PermissionWrite}, http.StatusNoContent},
		{"clear", `{"codes":[]}`, []string{}, http.StatusNoContent},
		{"missing", `{}`, nil, http.StatusUnprocessableEntity},
		{"null", `{"codes":null}`, nil, http.StatusUnprocessableEntity},
		{"unknown code", `{"codes":["admin"]}`, nil, http.StatusUnprocessableEntity},
		{"duplicate", `{"codes":["files:read","files:read"]}`, nil, http.StatusUnprocessableEntity},
		{"malformed", `{"codes":`, nil, http.StatusBadRequest},
		{"wrong type", `{"codes":"files:read"}`, nil, http.StatusBadRequest},
		{"unknown field", `{"codes":[],"extra":true}`, nil, http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, mock := newAuthenticatedTestApplication(t)
			expectPermissionsFile(mock, permissionOwnerID)
			if tt.status == http.StatusNoContent {
				mock.ExpectQuery("SELECT id, name, email, password_hash, activated FROM users WHERE id").
					WithArgs(data.UserID(permissionUserID)).
					WillReturnRows(sqlmock.NewRows([]string{"id", "name", "email", "password_hash", "activated"}).
						AddRow(permissionUserID, "Bob", "bob@example.com", []byte("!"), true))
				expectPermissionsReplacement(mock, tt.codes)
			}
			w := serveRequest(t, app, http.MethodPut, "/v1/files/"+permissionFileID+"/permissions/"+permissionUserID, []byte(tt.body))
			if w.Code != tt.status {
				t.Fatalf("got %d, want %d; body: %s", w.Code, tt.status, w.Body.String())
			}
			if tt.status == http.StatusNoContent && w.Body.Len() != 0 {
				t.Fatal("expected empty response")
			}
		})
	}
}

func TestPermissionsDeleteEndpoint(t *testing.T) {
	app, mock := newTestApplication(t)
	for range 2 {
		expectAuthentication(mock, true)
		expectPermissionsFile(mock, permissionOwnerID)
		expectPermissionsReplacement(mock, nil)
		w := serveRequest(t, app, http.MethodDelete, "/v1/files/"+permissionFileID+"/permissions/"+permissionUserID, nil)
		if w.Code != http.StatusNoContent || w.Body.Len() != 0 {
			t.Fatalf("got %d: %s", w.Code, w.Body.String())
		}
	}
}

func TestPermissionsEndpointErrors(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		for _, scenario := range []string{"not owner", "missing file", "file query failure", "invalid file ID", "invalid user ID", "storage failure", "missing user"} {
			if method == http.MethodGet && (scenario == "invalid user ID" || scenario == "missing user") || method == http.MethodDelete && scenario == "missing user" {
				continue
			}
			t.Run(method+"/"+scenario, func(t *testing.T) {
				app, mock := newAuthenticatedTestApplication(t)
				fileID, userID := permissionFileID, permissionUserID
				status := http.StatusInternalServerError
				switch scenario {
				case "invalid file ID":
					fileID, status = "invalid", http.StatusBadRequest
				case "missing file":
					mock.ExpectQuery("SELECT id, name, filetype, owner_id, status, size FROM files").WillReturnRows(sqlmock.NewRows([]string{"id", "name", "filetype", "owner_id", "status", "size"}))
					status = http.StatusNotFound
				case "file query failure":
					mock.ExpectQuery("SELECT id, name, filetype, owner_id, status, size FROM files").WillReturnError(errors.New("query failed"))
				case "not owner":
					expectPermissionsFile(mock, permissionUserID)
					status = http.StatusForbidden
				default:
					expectPermissionsFile(mock, permissionOwnerID)
					switch scenario {
					case "invalid user ID":
						userID, status = "invalid", http.StatusBadRequest
					case "missing user":
						mock.ExpectQuery("SELECT id, name, email, password_hash, activated FROM users WHERE id").WillReturnRows(sqlmock.NewRows([]string{"id", "name", "email", "password_hash", "activated"}))
						status = http.StatusNotFound
					case "storage failure":
						switch method {
						case http.MethodGet:
							mock.ExpectQuery("SELECT file_id, user_id, code FROM file_permissions").WillReturnError(errors.New("query failed"))
						case http.MethodPut:
							mock.ExpectQuery("SELECT id, name, email, password_hash, activated FROM users WHERE id").WillReturnError(errors.New("query failed"))
						case http.MethodDelete:
							mock.ExpectBegin().WillReturnError(errors.New("begin failed"))
						}
					}
				}
				target := "/v1/files/" + fileID + "/permissions"
				if method != http.MethodGet {
					target += "/" + userID
				}
				w := serveRequest(t, app, method, target, []byte(`{"codes":["files:read"]}`))
				assertJSONResponse(t, w, status)
			})
		}
	}
}
