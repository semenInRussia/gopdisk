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

	err := app.readJSON(r, &input)
	if err != nil {
		app.errorResponse(w, err.Error(), http.StatusBadRequest)
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
		app.errorResponse(w, err.Error(), http.StatusBadRequest)
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
	id, err := app.getIdParam(r)
	if err != nil {
		app.errorResponse(w, err.Error(), http.StatusBadRequest)
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
