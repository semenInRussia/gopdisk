package main

import (
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
	router.HandlerFunc(http.MethodGet, "/v1/users/:id", app.requireActivatedUser(app.usersGetHandler))
	router.HandlerFunc(http.MethodDelete, "/v1/users/:id", app.requireActivatedUser(app.usersDeleteHandler))
	router.HandlerFunc(http.MethodGet, "/v1/users/:id/files", app.requireActivatedUser(app.usersFilesGetHandler))
	router.HandlerFunc(http.MethodGet, "/v1/users/:id/uploads", app.requireActivatedUser(app.usersUploadsGetHandler))

	// Files
	router.HandlerFunc(http.MethodGet, "/v1/files/:id", app.requireActivatedUser(app.filesGetHandler))
	router.HandlerFunc(http.MethodDelete, "/v1/files/:id", app.requireActivatedUser(app.filesDeleteHandler))
	router.HandlerFunc(http.MethodPatch, "/v1/files/:id", app.requireActivatedUser(app.filesPatchHandler))
	router.HandlerFunc(http.MethodGet, "/v1/files/:id/content", app.requireActivatedUser(app.filesContentGetHandler))
	router.HandlerFunc(http.MethodHead, "/v1/files/:id/content", app.requireActivatedUser(app.filesContentHeadHandler))

	// Uploads
	router.HandlerFunc(http.MethodPost, "/v1/files/upload", app.requireActivatedUser(app.filesUploadsPostHandler))
	router.HandlerFunc(http.MethodGet, "/v1/uploads/:file_id", app.requireActivatedUser(app.uploadsGetHandler))
	router.HandlerFunc(http.MethodPut, "/v1/uploads/:file_id/parts/:position", app.requireActivatedUser(app.uploadsPartsPutHandler))
	router.HandlerFunc(http.MethodDelete, "/v1/uploads/:file_id/parts/:position", app.requireActivatedUser(app.uploadsPartsDeleteHandler))
	router.HandlerFunc(http.MethodPost, "/v1/uploads/:file_id/complete", app.requireActivatedUser(app.uploadsCompletePostHandler))
	router.HandlerFunc(http.MethodDelete, "/v1/uploads/:file_id", app.requireActivatedUser(app.uploadsDeleteHandler))

	// Tokens
	router.HandlerFunc(http.MethodPost, "/v1/tokens/authentication", app.tokensAuthenticationPostHandler)

	return app.recoverPanic(app.authenticate(router))
}
