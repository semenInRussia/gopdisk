package main

import (
	"encoding/json"
	"errors"
	"gopherdisk/internal/validator"
	"maps"
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"
)

type Envelope map[string]any

func (app *Application) readJSON(
	r *http.Request,
	result any,
) error {
	err := json.NewDecoder(r.Body).Decode(result)
	return err
}

func (app *Application) writeJSON(
	w http.ResponseWriter,
	data any,
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

func (app *Application) logError(err error) {
	app.logger.Println(err)
}

func (app *Application) errorResponse(w http.ResponseWriter, msg any, status int) {
	data := Envelope{
		"error": msg,
	}
	err := app.writeJSON(w, data, status, nil)
	if err != nil {
		app.logError(err)
		w.WriteHeader(500)
	}
}

func (app *Application) writeValidatorErrors(w http.ResponseWriter, v *validator.Validator) {
	status := http.StatusUnprocessableEntity
	if v.IsValid() {
		status = http.StatusOK
	}
	app.errorResponse(w, v.Errors, status)
}

func (app *Application) getIdParam(r *http.Request) (int64, error) {
	params := httprouter.ParamsFromContext(r.Context())
	id, err := strconv.ParseInt(params.ByName("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}
