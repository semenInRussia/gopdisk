package main

import (
	"errors"
	"gopherdisk/internal/data"
	"gopherdisk/internal/validator"
	"net/http"
)

func (app *Application) usersPostHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestErrorResponse(w, err.Error())
		return
	}

	v := validator.New()
	data.ValidateUserName(v, input.Name)
	input.Email = data.NormalizeEmail(input.Email)
	data.ValidateEmail(v, input.Email)
	data.ValidatePasswordPlaintext(v, input.Password)

	if !v.IsValid() {
		app.writeValidatorErrors(w, v)
		return
	}

	user, token, err := app.storage.Users.Register(r.Context(), input.Name, input.Email, input.Password)
	if err != nil {
		if errors.Is(err, data.ErrDuplicateEmail) {
			v.AddError("email", "a user with this email address already exists")
			app.writeValidatorErrors(w, v)
			return
		}
		app.serverErrorResponse(w, err)
		return
	}

	app.background(func() {
		templateData := map[string]any{
			"user":            user,
			"activationToken": token.Plaintext,
		}
		if err := app.mailer.Send(user.Email, "user_welcome.tmpl", templateData); err != nil {
			app.logError(err)
		}
	})

	response := Envelope{
		"message": "an email will be sent to you containing activation instructions",
	}

	err = app.writeJSON(w, response, http.StatusAccepted, nil)
	if err != nil {
		app.logError(err)
	}
}

func (app *Application) usersActivatedPutHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		TokenPlaintext string `json:"token"`
	}
	if err := app.readJSON(w, r, &input); err != nil {
		app.badRequestErrorResponse(w, err.Error())
		return
	}

	v := validator.New()
	data.ValidateTokenPlaintext(v, input.TokenPlaintext)
	if !v.IsValid() {
		app.writeValidatorErrors(w, v)
		return
	}

	user, err := app.storage.Users.Activate(r.Context(), input.TokenPlaintext)
	if err != nil {
		if errors.Is(err, data.ErrInvalidToken) {
			v.AddError("token", "is invalid or has expired")
			app.writeValidatorErrors(w, v)
			return
		}
		app.serverErrorResponse(w, err)
		return
	}

	if err := app.writeJSON(w, Envelope{"user": user}, http.StatusOK, nil); err != nil {
		app.serverErrorResponse(w, err)
	}
}

func (app *Application) usersMeGetHandler(w http.ResponseWriter, r *http.Request) {
	if err := app.writeJSON(w, Envelope{"user": app.contextGetUser(r)}, http.StatusOK, nil); err != nil {
		app.serverErrorResponse(w, err)
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
