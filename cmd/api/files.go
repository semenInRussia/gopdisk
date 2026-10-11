package main

import (
	"errors"
	"gopherdisk/internal/data"
	"gopherdisk/internal/validator"
	"net/http"
)

func (app *Application) filesGetHandler(w http.ResponseWriter, r *http.Request) {
	idStr, err := app.getIdParam(r)
	if err != nil {
		app.errorResponse(w, err.Error(), http.StatusBadRequest)
		return
	}

	id := data.FileID(idStr)

	file, err := app.storage.Files.Get(r.Context(), id)

	if err != nil {
		switch {
		case errors.Is(err, data.ErrFileNotFound):
			app.errorResponse(w, err.Error(), http.StatusNotFound)
		default:
			app.serverErrorResponse(w, err)
		}
		return
	}

	data := Envelope{
		"file": file,
	}

	err = app.writeJSON(w, data, http.StatusOK, nil)

	if err != nil {
		app.serverErrorResponse(w, err)
	}
}

// rename given file
func (app *Application) filesPatchHandler(w http.ResponseWriter, r *http.Request) {
	idStr, err := app.getIdParam(r)
	if err != nil {
		app.errorResponse(w, err.Error(), http.StatusBadRequest)
		return
	}

	id := data.FileID(idStr)

	var input struct {
		Name string
	}

	err = app.readJSON(r, &input)

	if err != nil {
		app.badRequestErrorResponse(w, err)
		return
	}

	val := validator.New()
	data.ValidateUserName(val, input.Name)

	if !val.IsValid() {
		app.writeValidatorErrors(w, val)
		return
	}

	file, err := app.storage.Files.Rename(r.Context(), id, input.Name)

	if err != nil {
		switch {
		case errors.Is(err, data.ErrFileNotFound):
			app.errorResponse(w, err.Error(), http.StatusNotFound)
		default:
			app.serverErrorResponse(w, err)
		}
		return
	}

	data := Envelope{"file": file}

	err = app.writeJSON(w, data, http.StatusOK, nil)

	if err != nil {
		app.serverErrorResponse(w, err)
	}
}

func (app *Application) filesUploadsPostHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		OwnerId  string `json:"owner_id"`
		Name     string
		FileType string `json:"file_type"`
		Size     int64  `json:"size"`
	}

	err := app.readJSON(r, &input)

	if err != nil {
		app.errorResponse(w, err.Error(), http.StatusBadRequest)
		return
	}

	file := data.File{
		OwnerID:  data.UserID(input.OwnerId),
		Name:     input.Name,
		Filetype: data.FileKind(input.FileType),
		Size:     input.Size,
	}

	err = app.storage.Files.Insert(r.Context(), &file)

	if err != nil {
		app.serverErrorResponse(w, err)
		return
	}

	data := Envelope{"file": file}

	app.writeJSON(w, data, http.StatusOK, nil)
}
