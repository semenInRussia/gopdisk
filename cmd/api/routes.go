package main

import (
	"encoding/json"
	"maps"
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func (app *Application) writeJSON(
	w http.ResponseWriter,
	data interface{},
	status int,
	headers http.Header,
) error {
	js, err := json.Marshal(data)
	if err != nil {
		return err
	}
	js = append(js, '\n')

	maps.Copy(w.Header(), headers)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(js)

	return nil
}

func (app *Application) mainHandler(w http.ResponseWriter, r *http.Request) {
	body := map[string]string{
		"hello": "seaman",
	}
	app.writeJSON(w, body, 203, nil)
}

func (app *Application) routes() *httprouter.Router {
	router := httprouter.New()

	router.HandlerFunc(http.MethodGet, "/v1/main", app.mainHandler)

	return router
}
