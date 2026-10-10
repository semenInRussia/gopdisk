package main

import (
	"errors"
	"gopherdisk/internal/data"
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
