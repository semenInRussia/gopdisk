package storage

import "errors"

var (
	ErrEarlyFinishUpload       = errors.New("Finishing uploading before each parst was upload")
	ErrIndexOutOfBounds        = errors.New("Index out of bounds")
	ErrInvalidPartSize         = errors.New("size of file part is invalid")
	ErrStorageDatabaseRequired = errors.New("Database connection is required")
	ErrStorageFileNotFound     = errors.New("Could not find a file in storage")
	ErrStorageInvalidSize      = errors.New("storage size must be non-negative")
	ErrUserNotFound            = errors.New("Could not find an user")
)
