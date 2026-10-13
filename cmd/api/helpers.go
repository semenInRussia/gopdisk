package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"gopherdisk/internal/validator"
	"maps"
	"net/http"

	"github.com/google/uuid"
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

func (app *Application) badRequestErrorResponse(w http.ResponseWriter, msg any) {
	app.errorResponse(w, msg, http.StatusBadRequest)
}

func (app *Application) serverErrorResponse(w http.ResponseWriter, err error) {
	app.logError(err)
	app.errorResponse(w, "the server encountered a problem and could not process your request", http.StatusInternalServerError)
}

func (app *Application) writeValidatorErrors(w http.ResponseWriter, v *validator.Validator) {
	status := http.StatusUnprocessableEntity
	if v.IsValid() {
		status = http.StatusOK
	}
	app.errorResponse(w, v.Errors, status)
}

func (app *Application) getIdParam(r *http.Request) (string, error) {
	params := httprouter.ParamsFromContext(r.Context())
	id := params.ByName("id")
	if id == "" {
		return "", errors.New("id is required, but not provided")
	}
	return id, nil
}

func (app *Application) getUUIDParam(r *http.Request, name string) (string, error) {
	params := httprouter.ParamsFromContext(r.Context())
	id := params.ByName(name)
	if id == "" {
		return "", fmt.Errorf("%s is required, but not provided", name)
	}
	if err := uuid.Validate(id); err != nil {
		return "", fmt.Errorf("%s must be a valid UUID: %w", name, err)
	}
	return id, nil
}
