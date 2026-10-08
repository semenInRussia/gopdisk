package main

import (
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
