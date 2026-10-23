package main

import (
	"context"
	"errors"
	"gopherdisk/internal/blobs"
	"gopherdisk/internal/data"
	"gopherdisk/internal/validator"
	"io"
	"net/http"
	"strconv"

	"github.com/google/uuid"
)

// get id from request params, find it in DB. Handler errors and return file and occured error (if can).
func (app *Application) getFileByPathID(w http.ResponseWriter, r *http.Request) (*data.File, error) {
	idStr, err := app.getUUIDParam(r, "id")
	if err != nil {
		app.badRequestErrorResponse(w, err.Error())
		return nil, err
	}

	id := data.FileID(idStr)
	file, err := app.storage.Files.Get(r.Context(), id)

	if err != nil {
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return nil, err
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
	idStr, err := app.getUUIDParam(r, "id")
	if err != nil {
		app.badRequestErrorResponse(w, err.Error())
		return
	}

	id := data.FileID(idStr)

	var input struct {
		Name string
	}

	err = app.readJSON(w, r, &input)

	if err != nil {
		app.badRequestErrorResponse(w, err.Error())
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
	idStr, err := app.getUUIDParam(r, "id")
	if err != nil {
		app.badRequestErrorResponse(w, err.Error())
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
		OwnerId string `json:"owner_id"`
		Name    string `json:"name"`
		Type    string `json:"type"`
		Size    int64  `json:"size"`
	}

	err := app.readJSON(w, r, &input)

	if err != nil {
		app.badRequestErrorResponse(w, err.Error())
		return
	}

	file := data.File{
		OwnerID: data.UserID(input.OwnerId),
		Name:    input.Name,
		Type:    data.FileType(input.Type),
		Size:    input.Size,
	}

	v := validator.New()
	data.ValidateFile(v, file)
	if !v.IsValid() {
		app.writeValidatorErrors(w, v)
		return
	}

	if uuid.MustParse(input.OwnerId).String() != string(app.contextGetUser(r).ID) {
		app.notPermittedResponse(w)
		return
	}
	file.OwnerID = app.contextGetUser(r).ID

	err = app.storage.Files.Insert(r.Context(), &file)

	if err != nil {
		app.serverErrorResponse(w, err)
		return
	}

	data := Envelope{
		"file":      file,
		"part_size": blobs.BlobsMaxSize,
	}

	if err := app.writeJSON(w, data, http.StatusOK, nil); err != nil {
		app.serverErrorResponse(w, err)
	}
}

func (app *Application) uploadsGetHandler(w http.ResponseWriter, r *http.Request) {
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

	data := Envelope{"upload": upload}
	err = app.writeJSON(w, data, http.StatusOK, nil)
	if err != nil {
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
	idStr, err := app.getUUIDParam(r, "id")
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
	w.Header().Set("Content-Type", string(file.Type))
	w.Header().Set("Content-Length", strconv.FormatInt(file.Size, 10))
}

func (app *Application) uploadsPartsPutHandler(w http.ResponseWriter, r *http.Request) {
	fileIDStr, err := app.getUUIDParam(r, "file_id")
	if err != nil {
		app.badRequestErrorResponse(w, err.Error())
		return
	}

	fileID := data.FileID(fileIDStr)

	pos, err := app.getInt64Param(r, "position")
	if err != nil {
		app.badRequestErrorResponse(w, err.Error())
		return
	}

	_, err = app.storage.Files.GetOneBlob(r.Context(), fileID, int(pos))
	existsBefore := err == nil

	blob, err := app.storage.Files.AddReader(r.Context(), fileID, int(pos), r.Body)

	if err != nil {
		switch {
		case errors.Is(err, data.ErrFileNotFound):
			app.errorResponse(w, err.Error(), http.StatusNotFound)
		case errors.Is(err, data.ErrInvalidPosition):
			app.badRequestErrorResponse(w, "got invalid position to upload")
		case errors.Is(err, io.EOF):
			app.badRequestErrorResponse(w, "cannot upload an empty part")
		default:
			app.serverErrorResponse(w, err)
		}
		return
	}

	data := Envelope{
		"part": data.UploadPart{
			Position: int(pos),
			Size:     blob.Size,
		},
	}

	status := http.StatusCreated
	if existsBefore {
		status = http.StatusOK
	}

	err = app.writeJSON(w, data, status, nil)
	if err != nil {
		app.serverErrorResponse(w, err)
	}
}

func (app *Application) uploadsPartsDeleteHandler(w http.ResponseWriter, r *http.Request) {
	fileID, err := app.getUUIDParam(r, "file_id")
	if err != nil {
		app.badRequestErrorResponse(w, err.Error())
		return
	}

	position, err := app.getInt64Param(r, "position")
	if err != nil {
		app.badRequestErrorResponse(w, err.Error())
		return
	}

	err = app.storage.Files.RemoveBlob(r.Context(), data.FileID(fileID), int(position))
	if err != nil {
		switch {
		case errors.Is(err, data.ErrFileNotFound), errors.Is(err, data.ErrBlobNotFound):
			app.errorResponse(w, err.Error(), http.StatusNotFound)
		case errors.Is(err, data.ErrInvalidPosition):
			app.badRequestErrorResponse(w, "got invalid position to remove")
		default:
			app.serverErrorResponse(w, err)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *Application) uploadsCompletePostHandler(w http.ResponseWriter, r *http.Request) {
	fileID, err := app.getUUIDParam(r, "file_id")
	if err != nil {
		app.badRequestErrorResponse(w, err.Error())
		return
	}

	id := data.FileID(fileID)
	if err := app.storage.Files.Complete(r.Context(), id); err != nil {
		switch {
		case errors.Is(err, data.ErrEarlyComplete):
			app.errorResponse(w, err.Error(), http.StatusConflict)
		case errors.Is(err, data.ErrFileNotFound):
			file, getErr := app.storage.Files.Get(r.Context(), id)
			if getErr == nil {
				data := Envelope{
					"file": file,
				}
				err := app.writeJSON(w, data, http.StatusOK, nil)
				if err != nil {
					app.serverErrorResponse(w, err)
				}
				return
			}
			if errors.Is(getErr, data.ErrFileNotFound) {
				app.errorResponse(w, err.Error(), http.StatusNotFound)
				return
			}
			app.serverErrorResponse(w, getErr)
		default:
			app.serverErrorResponse(w, err)
		}
		return
	}

	file, err := app.storage.Files.Get(r.Context(), id)
	if err != nil {
		app.serverErrorResponse(w, err)
		return
	}

	err = app.writeJSON(w, Envelope{"file": file}, http.StatusOK, nil)
	if err != nil {
		app.serverErrorResponse(w, err)
	}
}

func (app *Application) uploadsDeleteHandler(w http.ResponseWriter, r *http.Request) {
	fileID, err := app.getUUIDParam(r, "file_id")
	if err != nil {
		app.badRequestErrorResponse(w, err.Error())
		return
	}

	err = app.storage.Files.CancelUpload(r.Context(), data.FileID(fileID))
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
