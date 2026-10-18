package main

import (
	"gopherdisk/internal/data"
	"net/http"
	"time"

	"github.com/julienschmidt/httprouter"
)

func (app *Application) healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	body := Envelope{
		"status":  "available",
		"version": version,
		"time":    time.Now(),
	}
	app.writeJSON(w, body, http.StatusNonAuthoritativeInfo, nil)
}

func (app *Application) routes() http.Handler {
	router := httprouter.New()

	router.MethodNotAllowed = http.HandlerFunc(app.methodNotAllowedResponse)
	router.NotFound = http.HandlerFunc(app.notFoundResponse)

	router.HandlerFunc(http.MethodGet, "/v1/healthcheck", app.healthCheckHandler)

	// Users
	router.HandlerFunc(http.MethodPost, "/v1/users", app.usersPostHandler)
	router.HandlerFunc(http.MethodPut, "/v1/users/activated", app.usersActivatedPutHandler)
	router.HandlerFunc(http.MethodGet, "/v1/users/:id", app.requireCurrentUser(app.usersGetHandler))
	router.HandlerFunc(http.MethodDelete, "/v1/users/:id", app.requireCurrentUser(app.usersDeleteHandler))
	router.HandlerFunc(http.MethodGet, "/v1/users/:id/files", app.requireCurrentUser(app.usersFilesGetHandler))
	router.HandlerFunc(http.MethodGet, "/v1/users/:id/uploads", app.requireCurrentUser(app.usersUploadsGetHandler))

	// Files
	router.HandlerFunc(http.MethodGet, "/v1/files/:id", app.requireFilePermission("id", data.PermissionRead, app.filesGetHandler))
	router.HandlerFunc(http.MethodDelete, "/v1/files/:id", app.requireFilePermission("id", data.PermissionDelete, app.filesDeleteHandler))
	router.HandlerFunc(http.MethodPatch, "/v1/files/:id", app.requireFilePermission("id", data.PermissionWrite, app.filesPatchHandler))
	router.HandlerFunc(http.MethodGet, "/v1/files/:id/content", app.requireFilePermission("id", data.PermissionRead, app.filesContentGetHandler))
	router.HandlerFunc(http.MethodHead, "/v1/files/:id/content", app.requireFilePermission("id", data.PermissionRead, app.filesContentHeadHandler))

	// Uploads
	router.HandlerFunc(http.MethodPost, "/v1/files/upload", app.requireActivatedUser(app.filesUploadsPostHandler))
	router.HandlerFunc(http.MethodGet, "/v1/uploads/:file_id", app.requireFilePermission("file_id", data.PermissionRead, app.uploadsGetHandler))
	router.HandlerFunc(http.MethodPut, "/v1/uploads/:file_id/parts/:position", app.requireFilePermission("file_id", data.PermissionWrite, app.uploadsPartsPutHandler))
	router.HandlerFunc(http.MethodDelete, "/v1/uploads/:file_id/parts/:position", app.requireFilePermission("file_id", data.PermissionWrite, app.uploadsPartsDeleteHandler))
	router.HandlerFunc(http.MethodPost, "/v1/uploads/:file_id/complete", app.requireFilePermission("file_id", data.PermissionWrite, app.uploadsCompletePostHandler))
	router.HandlerFunc(http.MethodDelete, "/v1/uploads/:file_id", app.requireFilePermission("file_id", data.PermissionDelete, app.uploadsDeleteHandler))

	// File permissions
	router.HandlerFunc(http.MethodGet, "/v1/files/:id/permissions", app.requireActivatedUser(app.permissionsGetHandler))
	router.HandlerFunc(http.MethodPut, "/v1/files/:id/permissions/:user_id", app.requireActivatedUser(app.permissionsPutHandler))
	router.HandlerFunc(http.MethodDelete, "/v1/files/:id/permissions/:user_id", app.requireActivatedUser(app.permissionsDeleteHandler))

	// Tokens
	router.HandlerFunc(http.MethodPost, "/v1/tokens/authentication", app.tokensAuthenticationPostHandler)

	return app.recoverPanic(app.authenticate(router))
}
