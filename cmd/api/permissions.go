package main

import (
	"errors"
	"gopherdisk/internal/data"
	"gopherdisk/internal/validator"
	"net/http"
)

// permissionsFile checks ownership only for permissions management endpoints.
func (app *Application) permissionsFile(w http.ResponseWriter, r *http.Request) (*data.File, bool) {
	id, err := app.getUUIDParam(r, "id")
	if err != nil {
		app.badRequestErrorResponse(w, err.Error())
		return nil, false
	}
	file, err := app.storage.Files.Get(r.Context(), data.FileID(id))
	if err != nil {
		app.permissionsErrorResponse(w, r, err)
		return nil, false
	}
	if file.OwnerID != app.contextGetUser(r).ID {
		app.notPermittedResponse(w)
		return nil, false
	}
	return file, true
}

func (app *Application) permissionsErrorResponse(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, data.ErrFileNotFound) || errors.Is(err, data.ErrUserNotFound) {
		app.notFoundResponse(w, r)
		return
	}
	app.serverErrorResponse(w, err)
}

func (app *Application) permissionsGetHandler(w http.ResponseWriter, r *http.Request) {
	file, ok := app.permissionsFile(w, r)
	if !ok {
		return
	}
	permissions, err := app.storage.Permissions.GetAllForFile(r.Context(), file.ID)
	if err != nil {
		app.permissionsErrorResponse(w, r, err)
		return
	}
	if err := app.writeJSON(w, Envelope{"permissions": permissions}, http.StatusOK, nil); err != nil {
		app.serverErrorResponse(w, err)
	}
}

func (app *Application) permissionsPutHandler(w http.ResponseWriter, r *http.Request) {
	file, ok := app.permissionsFile(w, r)
	if !ok {
		return
	}
	userID, err := app.getUUIDParam(r, "user_id")
	if err != nil {
		app.badRequestErrorResponse(w, err.Error())
		return
	}
	var input struct {
		Codes []string `json:"codes"`
	}
	if err := app.readJSON(w, r, &input); err != nil {
		app.badRequestErrorResponse(w, err.Error())
		return
	}
	v := validator.New()
	data.ValidatePermissionCodes(v, input.Codes)
	if !v.IsValid() {
		app.writeValidatorErrors(w, v)
		return
	}
	if _, err := app.storage.Users.Get(r.Context(), data.UserID(userID)); err != nil {
		app.permissionsErrorResponse(w, r, err)
		return
	}
	if err := app.storage.Permissions.SetForUser(r.Context(), file.ID, data.UserID(userID), input.Codes); err != nil {
		app.permissionsErrorResponse(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (app *Application) permissionsDeleteHandler(w http.ResponseWriter, r *http.Request) {
	file, ok := app.permissionsFile(w, r)
	if !ok {
		return
	}
	userID, err := app.getUUIDParam(r, "user_id")
	if err != nil {
		app.badRequestErrorResponse(w, err.Error())
		return
	}
	if err := app.storage.Permissions.DeleteForUser(r.Context(), file.ID, data.UserID(userID)); err != nil {
		app.permissionsErrorResponse(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
