package main

import (
	"errors"
	"gopherdisk/internal/data"
	"gopherdisk/internal/validator"
	"net/http"
	"time"
)

func (app *Application) tokensAuthenticationPostHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email             string `json:"email"`
		PasswordPlaintext string `json:"password"`
	}

	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestErrorResponse(w, err.Error())
		return
	}

	v := validator.New()
	data.ValidateEmail(v, input.Email)
	input.Email = data.NormalizeEmail(input.Email)

	if !v.IsValid() {
		app.writeValidatorErrors(w, v)
		return
	}

	user, err := app.storage.Users.GetByEmail(r.Context(), input.Email)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrUserNotFound):
			app.invalidCredentialsResponse(w)
		default:
			app.serverErrorResponse(w, err)
		}
		return
	}

	if !user.Activated {
		app.invalidCredentialsResponse(w)
		return
	}

	match, err := user.Password.Matches(input.PasswordPlaintext)
	if err != nil {
		app.serverErrorResponse(w, err)
		return
	}

	if !match {
		app.invalidCredentialsResponse(w)
		return
	}

	token, err := app.storage.Tokens.New(r.Context(), user.ID, 1*24*time.Hour, data.ScopeAuthentication)
	if err != nil {
		app.serverErrorResponse(w, err)
		return
	}

	data := Envelope{
		"authentication_token": token,
	}

	err = app.writeJSON(w, data, http.StatusCreated, nil)
	if err != nil {
		app.serverErrorResponse(w, err)
	}
}
