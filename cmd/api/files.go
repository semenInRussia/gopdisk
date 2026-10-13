package main

import (
	"context"
	"errors"
	"gopherdisk/internal/data"
	"gopherdisk/internal/validator"
	"io"
	"net/http"
	"strconv"
)

// get id from request params, find it in DB. Handler errors and return file and occured error (if can).
func (app *Application) getFileByPathID(w http.ResponseWriter, r *http.Request) (*data.File, error) {
	idStr, err := app.getIdParam(r)
	if err != nil {
		app.badRequestErrorResponse(w, err)
		return nil, err
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
		return nil, err
	}

	return file, nil
}

func (app *Application) filesGetHandler(w http.ResponseWriter, r *http.Request) {
	file, err := app.getFileByPathID(w, r)
	if err != nil {
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
		app.badRequestErrorResponse(w, err)
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

func (app *Application) filesDeleteHandler(w http.ResponseWriter, r *http.Request) {
	idStr, err := app.getIdParam(r)
	if err != nil {
		app.badRequestErrorResponse(w, err)
		return
	}

	id := data.FileID(idStr)

	err = app.storage.Files.Delete(r.Context(), id)

	if err != nil {
		switch {
		case errors.Is(err, data.ErrFileNotFound):
			app.errorResponse(w, err.Error(), http.StatusNotFound)
		default:
			app.serverErrorResponse(w, err)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *Application) filesUploadsPostHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		OwnerId  string `json:"owner_id"`
		Name     string `json:"name"`
		FileType string `json:"file_type"`
		Size     int64  `json:"size"`
	}

	err := app.readJSON(r, &input)

	if err != nil {
		app.badRequestErrorResponse(w, err)
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

func (app *Application) filesUploadGetHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.getUUIDParam(r, "file_id")
	if err != nil {
		app.badRequestErrorResponse(w, err.Error())
		return
	}

	upload, err := app.storage.Files.GetUpload(r.Context(), data.FileID(id))
	if err != nil {
		switch {
		case errors.Is(err, data.ErrFileNotFound):
			app.errorResponse(w, err.Error(), http.StatusNotFound)
		default:
			app.serverErrorResponse(w, err)
		}
		return
	}

	if err := app.writeJSON(w, Envelope{"upload": upload}, http.StatusOK, nil); err != nil {
		app.serverErrorResponse(w, err)
	}
}

func (app *Application) filesContentGetHandler(w http.ResponseWriter, r *http.Request) {
	file, err := app.getFileByPathID(w, r)
	if err != nil {
		return
	}

	dataCh, errCh := app.storage.Files.Read(r.Context(), file.ID)

	setFileContentHeaders(w, file)

	responseStarted := false

	for chunk := range dataCh {
		responseStarted = true
		n, writeErr := w.Write(chunk)
		if writeErr != nil {
			app.logError(writeErr)
			return
		}
		if n != len(chunk) {
			app.logError(io.ErrShortWrite)
			return
		}
	}

	for err := range errCh {
		if err == nil {
			continue
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		if responseStarted {
			app.logError(err)
			return
		}
		if errors.Is(err, data.ErrFileNotFound) {
			app.errorResponse(w, err.Error(), http.StatusNotFound)
			return
		}
		app.serverErrorResponse(w, err)
		return
	}
}

func (app *Application) filesContentHeadHandler(w http.ResponseWriter, r *http.Request) {
	idStr, err := app.getIdParam(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	file, err := app.storage.Files.Get(r.Context(), data.FileID(idStr))
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return
		case errors.Is(err, data.ErrFileNotFound):
			w.WriteHeader(http.StatusNotFound)
		default:
			app.logError(err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	setFileContentHeaders(w, file)
	w.WriteHeader(http.StatusOK)
}

func setFileContentHeaders(w http.ResponseWriter, file *data.File) {
	w.Header().Set("Content-Type", string(file.Filetype))
	w.Header().Set("Content-Length", strconv.FormatInt(file.Size, 10))
}
