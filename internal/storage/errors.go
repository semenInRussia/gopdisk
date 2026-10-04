package storage

import "errors"

var (
	ErrEarlyFinishUpload       = errors.New("Finishing uploading before each parst was upload")
	ErrIndexOutOfBounds        = errors.New("Index out of bounds")
	ErrStorageDatabaseRequired = errors.New("Database connection is required")
	ErrStorageFileNotFound     = errors.New("Could not find a file in storage")
	ErrUserNotFound            = errors.New("Could not find an user")
)
