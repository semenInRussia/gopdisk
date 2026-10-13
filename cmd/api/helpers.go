package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"gopherdisk/internal/validator"
	"io"
	"maps"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/julienschmidt/httprouter"
)

type Envelope map[string]any

func (app *Application) readJSON(
	w http.ResponseWriter,
	r *http.Request,
	result any,
) error {
	maxBytes := 1_048_576
	r.Body = http.MaxBytesReader(w, r.Body, int64(maxBytes))

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	err := dec.Decode(result)

	if err != nil {
		var syntaxError *json.SyntaxError
		var unmarshalTypeError *json.UnmarshalTypeError
		var invalidUnmarshalError *json.InvalidUnmarshalError

		switch {
		case errors.As(err, &syntaxError):
			return fmt.Errorf("body contains badly-formed JSON (at character %d)", syntaxError.Offset)

		case errors.Is(err, io.ErrUnexpectedEOF):
			return errors.New("body contains badly-formed JSON")

		case errors.As(err, &unmarshalTypeError):
			if unmarshalTypeError.Field != "" {
				return fmt.Errorf("body contains incorrect JSON type for field %q", unmarshalTypeError.Field)
			}
			return fmt.Errorf("body contains incorrect JSON type (at character %d)", unmarshalTypeError.Offset)

		case errors.Is(err, io.EOF):
			return errors.New("body must not be empty")

		case strings.HasPrefix(err.Error(), "json: unknown field "):
			fieldName := strings.TrimPrefix(err.Error(), "json: unknown field ")
			return fmt.Errorf("body contains unknown key %s", fieldName)

		case err.Error() == "http: request body too large":
			return fmt.Errorf("body must not be larger than %d bytes", maxBytes)

		case errors.As(err, &invalidUnmarshalError):
			panic(err)

		default:
			return err
		}
	}

	err = dec.Decode(&struct{}{})
	if err != io.EOF {
		return errors.New("body must only contain a single JSON value")
	}

	return nil
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

func (app *Application) getInt64Param(r *http.Request, name string) (int64, error) {
	params := httprouter.ParamsFromContext(r.Context())
	numStr := params.ByName(name)
	num, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cannot read number from params: %v", err)
	}
	return num, nil
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
