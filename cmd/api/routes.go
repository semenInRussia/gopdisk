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
	app.writeJSON(w, body, 203, nil)
}

func (app *Application) routes() http.Handler {
	router := httprouter.New()

	router.HandlerFunc(http.MethodGet, "/v1/healthcheck", app.healthCheckHandler)

	// Users
	router.HandlerFunc(http.MethodPost, "/v1/users", app.usersPostHandler)
	router.HandlerFunc(http.MethodGet, "/v1/users/:id", app.usersGetHandler)

	// Files
	router.HandlerFunc(http.MethodGet, "/v1/files/:id", app.filesGetHandler)

	return app.recoverPanic(router)
}
