package data

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"testing"

	"gopherdisk/internal/validator"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestValidatePermissionCodes(t *testing.T) {
	for _, tt := range []struct {
		name  string
		codes []string
		valid bool
	}{
		{"missing", nil, false},
		{"empty set", []string{}, true},
		{"all codes", []string{PermissionRead, PermissionWrite, PermissionDelete}, true},
		{"unknown", []string{"admin"}, false},
		{"duplicate", []string{PermissionRead, PermissionRead}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := validator.New()
			ValidatePermissionCodes(v, tt.codes)
			if v.IsValid() != tt.valid {
				t.Fatalf("valid=%v, want %v: %v", v.IsValid(), tt.valid, v.Errors)
			}
		})
	}
}

func TestPermissionsGetAllForFile(t *testing.T) {
	for _, scenario := range []string{"rows", "empty", "query error", "scan error", "iteration error"} {
		t.Run(scenario, func(t *testing.T) {
			files, mock := newFileModelMock(t)
			m := PermissionsModel{db: files.db}
			rows := sqlmock.NewRows([]string{"file_id", "user_id", "code"})
			if scenario == "rows" {
				rows.AddRow("file", "alice", PermissionRead).AddRow("file", "bob", PermissionWrite)
			}
			if scenario == "scan error" {
				rows.AddRow("file", "alice", nil)
			}
			if scenario == "iteration error" {
				rows.AddRow("file", "alice", PermissionRead).RowError(0, errors.New("read failed"))
			}
			expectation := mock.ExpectQuery("SELECT file_id, user_id, code FROM file_permissions").WithArgs(FileID("file"))
			if scenario == "query error" {
				expectation.WillReturnError(errors.New("query failed"))
			} else {
				expectation.WillReturnRows(rows).RowsWillBeClosed()
			}
			got, err := m.GetAllForFile(context.Background(), "file")
			if scenario == "rows" || scenario == "empty" {
				if err != nil || got == nil {
					t.Fatalf("got %v, %v", got, err)
				}
				want := []FilePermission{}
				if scenario == "rows" {
					want = []FilePermission{{"file", "alice", PermissionRead}, {"file", "bob", PermissionWrite}}
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("got %v, want %v", got, want)
				}
			} else if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestPermissionsSetForUser(t *testing.T) {
	failure := errors.New("database failure")
	for _, scenario := range []string{"replace", "empty set", "delete", "begin error", "missing file", "lock error", "delete error", "second insert error", "missing user", "commit error"} {
		t.Run(scenario, func(t *testing.T) {
			files, mock := newFileModelMock(t)
			m := PermissionsModel{db: files.db}
			codes := []string{PermissionRead, PermissionWrite}
			if scenario == "empty set" || scenario == "delete" {
				codes = []string{}
			}
			begin := mock.ExpectBegin()
			if scenario == "begin error" {
				begin.WillReturnError(failure)
			} else {
				lock := mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM files WHERE id = $1 FOR UPDATE")).WithArgs(FileID("file"))
				switch scenario {
				case "missing file":
					lock.WillReturnRows(sqlmock.NewRows([]string{"id"}))
				case "lock error":
					lock.WillReturnError(failure)
				default:
					lock.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("file"))
					del := mock.ExpectExec("DELETE FROM file_permissions").WithArgs(FileID("file"), UserID("user"))
					if scenario == "delete error" {
						del.WillReturnError(failure)
					} else {
						del.WillReturnResult(sqlmock.NewResult(0, 2))
						for i, code := range codes {
							insert := mock.ExpectExec("INSERT INTO file_permissions").WithArgs(FileID("file"), UserID("user"), code)
							if scenario == "missing user" {
								insert.WillReturnError(&pgconn.PgError{Code: "23503", ConstraintName: "file_permissions_user_id_fkey"})
								break
							}
							if scenario == "second insert error" && i == 1 {
								insert.WillReturnError(failure)
								break
							}
							insert.WillReturnResult(sqlmock.NewResult(0, 1))
						}
					}
				}
				switch scenario {
				case "replace", "empty set", "delete":
					mock.ExpectCommit()
				case "commit error":
					mock.ExpectCommit().WillReturnError(failure)
				default:
					mock.ExpectRollback()
				}
			}
			var err error
			if scenario == "delete" {
				err = m.DeleteForUser(context.Background(), "file", "user")
			} else {
				err = m.SetForUser(context.Background(), "file", "user", codes)
			}
			var want error
			switch scenario {
			case "replace", "empty set", "delete":
			case "missing file":
				want = ErrFileNotFound
			case "missing user":
				want = ErrUserNotFound
			default:
				want = failure
			}
			if !errors.Is(err, want) {
				t.Fatalf("got %v, want %v", err, want)
			}
		})
	}
}
