package main

import (
	"errors"
	"gopherdisk/internal/data"
	"gopherdisk/internal/validator"
	"net/http"
)

func (app *Application) usersPostHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name"`
	}

	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestErrorResponse(w, err.Error())
		return
	}

	v := validator.New()
	data.ValidateUserName(v, input.Name)

	if !v.IsValid() {
		app.writeValidatorErrors(w, v)
		return
	}

	user, err := app.storage.Users.New(r.Context(), input.Name)
	if err != nil {
		app.badRequestErrorResponse(w, err.Error())
		return
	}

	data := Envelope{
		"user": user,
	}

	err = app.writeJSON(w, data, http.StatusCreated, nil)
	if err != nil {
		app.logError(err)
	}
}

func (app *Application) usersGetHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.getUUIDParam(r, "id")
	if err != nil {
		app.badRequestErrorResponse(w, err.Error())
		return
	}

	user, err := app.storage.Users.Get(r.Context(), data.UserID(id))
	if err != nil {
		switch {
		case errors.Is(err, data.ErrUserNotFound):
			app.errorResponse(w, err.Error(), http.StatusNotFound)
		default:
			app.errorResponse(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	data := Envelope{
		"user": user,
	}

	err = app.writeJSON(w, data, http.StatusOK, nil)
	if err != nil {
		app.logError(err)
	}
}

func (app *Application) usersDeleteHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.getUUIDParam(r, "id")
	if err != nil {
		app.badRequestErrorResponse(w, err.Error())
		return
	}

	err = app.storage.Users.Delete(r.Context(), data.UserID(id))
	if err != nil {
		switch {
		case errors.Is(err, data.ErrUserNotFound):
			app.errorResponse(w, err.Error(), http.StatusNotFound)
		default:
			app.errorResponse(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *Application) usersFilesGetHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.getUUIDParam(r, "id")
	if err != nil {
		app.badRequestErrorResponse(w, err.Error())
		return
	}

	files, err := app.storage.Users.GetFiles(r.Context(), data.UserID(id))
	if err != nil {
		switch {
		case errors.Is(err, data.ErrUserNotFound):
			app.badRequestErrorResponse(w, err.Error())
		default:
			app.serverErrorResponse(w, err)
		}
		return
	}

	data := Envelope{
		"files": files,
	}

	err = app.writeJSON(w, data, http.StatusOK, nil)

	if err != nil {
		app.serverErrorResponse(w, err)
	}
}

func (app *Application) usersUploadsGetHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.getUUIDParam(r, "id")
	if err != nil {
		app.badRequestErrorResponse(w, err.Error())
		return
	}

	files, err := app.storage.Users.GetUploads(r.Context(), data.UserID(id))
	if err != nil {
		switch {
		case errors.Is(err, data.ErrUserNotFound):
			app.badRequestErrorResponse(w, err.Error())
		default:
			app.serverErrorResponse(w, err)
		}
		return
	}

	data := Envelope{
		"files": files,
	}

	err = app.writeJSON(w, data, http.StatusOK, nil)

	if err != nil {
		app.serverErrorResponse(w, err)
	}
}
