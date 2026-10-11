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

	router.HandlerFunc(http.MethodGet, "/v1/healthcheck", app.healthCheckHandler)

	// Users
	router.HandlerFunc(http.MethodPost, "/v1/users", app.usersPostHandler)
	router.HandlerFunc(http.MethodGet, "/v1/users/:id", app.usersGetHandler)
	router.HandlerFunc(http.MethodDelete, "/v1/users/:id", app.usersDeleteHandler)
	router.HandlerFunc(http.MethodGet, "/v1/users/:id/files", app.usersFilesGetHandler)
	router.HandlerFunc(http.MethodGet, "/v1/users/:id/uploads", app.usersUploadsGetHandler)

	// Files
	router.HandlerFunc(http.MethodGet, "/v1/files/:id", app.filesGetHandler)
	router.HandlerFunc(http.MethodDelete, "/v1/files/:id", app.filesDeleteHandler)
	router.HandlerFunc(http.MethodPatch, "/v1/files/:id", app.filesPatchHandler)
	router.HandlerFunc(http.MethodGet, "/v1/files/:id/content", app.filesContentGetHandler)
	router.HandlerFunc(http.MethodPost, "/v1/files/upload", app.filesUploadsPostHandler)

	return app.recoverPanic(router)
}
