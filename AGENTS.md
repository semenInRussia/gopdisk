# AGENTS.md

## Project

GopherDisk is a local file storage system written in Go.

The goal of the project is to build a production-like storage backend from
the ground up. The current stage focuses on the core storage/domain logic
with PostgreSQL-backed metadata.

---

## Current architecture

The project consists of two main layers:

- `internal/storage` — high-level storage/domain logic
- `internal/blobs` — low-level physical blob storage

### storage

`Storage` manages:

- users
- files
- file metadata
- relationships between files and physical blobs

Main domain types:

```go
type FileID string
type UserID string
type FileType string
type FileStatus string

type User struct {
    ID   UserID
    Name string
}

type File struct {
    ID      FileID
    Name    string
    Type    FileType
    OwnerID UserID
    Status  FileStatus
    Blobs   []blobs.Blob
}
```

````

`Storage` currently contains:

```go
type Storage struct {
    bs blobs.BlobStorage
    db *sql.DB
}
```

### blobs

`BlobStorage` is responsible only for physical data.

A large user file can be split into multiple blobs.

Each blob:

- is stored as a separate physical file on the filesystem
- has a SHA-256 hash
- has a size
- has a maximum size (`BlobsMaxSize`, currently 16 MB)

```go
type Blob struct {
    Hash string
    Size int
}
```

`storage.File.Blobs` stores the ordered list of blobs belonging to the
logical file.

The order of blobs is important and MUST NOT be lost.

---

## Important design principle

A logical `File` is NOT a physical file.

A logical file may consist of many physical blobs:

```text
File
 ├── Blob 0
 ├── Blob 1
 ├── Blob 2
 └── ...
```

The filesystem stores blobs, while PostgreSQL stores the logical metadata.

Do not collapse this abstraction.

---

## Large file handling

Files larger than `BlobsMaxSize` must be split into chunks.

For example:

```text
100 MB file

        File
         |
   +-----+-----+-----+-----+
   |     |     |     |     |
 16MB  16MB  16MB  16MB  ...
```

Uploading chunks may happen concurrently.

Reading a file should produce its chunks in the original order.

The public `Read` interface currently returns:

```go
(<-chan []byte, <-chan error)
```

The data channel represents chunks of the logical file.

Consumers should be able to reconstruct the original file by reading
chunks in order.

---

## Concurrency

Concurrency is an important part of this project.

Uploading independent blobs may happen concurrently.

However:

- shared state must be protected from data races
- errors from goroutines must not be lost
- blob ordering must remain deterministic
- goroutines must not leak
- channels must be closed correctly
- use `go test -race` when changing concurrent code

Do NOT introduce concurrency just for the sake of concurrency.

If an operation can safely run concurrently because its parts are independent,
concurrency is encouraged.

---

## Error handling

Use explicit package-level sentinel errors where appropriate.

Example:

```go
var ErrStorageFileNotFound = errors.New("file not found")
```

Prefer:

```go
errors.Is(err, ErrStorageFileNotFound)
```

Do not silently ignore errors.

Never overwrite a shared error variable from multiple goroutines without
synchronization.

---

## Blob storage rules

`BlobStorage` owns the filesystem operations.

`Storage` should not directly manipulate blob files.

Use the blob layer for:

- writing blobs
- reading blobs
- deleting blobs
- splitting data into chunks
- computing blob hashes

The `storage` layer should work with `blobs.Blob` metadata instead of
filesystem paths.

---

## File lifecycle

Uploading a file conceptually consists of:

1. create logical file metadata
2. split input data into chunks
3. store chunks as blobs
4. associate blobs with the logical file
5. make the file available through `Storage`

If an upload fails, the implementation should avoid leaving an invalid
logical file in storage.

Partial physical blobs should eventually be cleaned up.

Do not implement complex transactional infrastructure unless it is actually
needed.

---

## Delete semantics

Deleting a logical file should eventually remove:

1. logical file metadata
2. physical blobs that are no longer referenced

Be aware that blobs are identified by content hash.

In the future, multiple logical files may reference the same blob.

Therefore blob deletion should eventually account for blob references.

Do not assume that a blob can always be deleted immediately when one file
is deleted.

---

## Update semantics

Renaming a file changes only metadata.

Updating file contents is different:

```text
old File
   |
   +-- old blobs

new contents
   |
   +-- new blobs
```

The implementation must avoid corrupting the logical file if writing the new
contents fails.

Do not modify the existing blob list until the new data has been successfully
stored.

---

## IDs

Use UUIDs for `UserID` and `FileID`.

ID generation should be encapsulated in functions such as:

```go
GenerateUserID()
GenerateFileID()
```

---

## Testing

Every important behavior should have tests.

Tests should focus on behavior rather than implementation details.

Important cases include:

- creating users
- unique user IDs
- uploading small files
- uploading files larger than one blob
- reading files
- preserving chunk order
- deleting files
- renaming files
- updating file contents
- nonexistent files
- empty files
- files exactly `BlobsMaxSize`
- files just above `BlobsMaxSize`
- filesystem errors
- concurrent uploads

Use:

```bash
go test ./...
```

and for concurrent code:

```bash
go test -race ./...
```

Tests must not depend on the developer's filesystem.

Use `t.TempDir()` for temporary storage.

Do not hardcode absolute paths such as:

```text
/Users/...
```

Test data should live inside the repository, for example:

```text
testdata/
```

---

## Code style

Follow standard Go conventions.

Run:

```bash
gofmt
go vet ./...
go test ./...
```

Prefer simple Go code over abstractions that are not currently needed.

Do not introduce interfaces unless they provide a real architectural benefit,
make testing easier, or represent a meaningful boundary.

Avoid unnecessary getters/setters and boilerplate.

Prefer small functions with clear responsibilities.

---

## Current scope

The current development stage is ONLY the core storage engine.

Do NOT add:

- HTTP API
- REST
- gRPC
- Redis
- Kafka
- Kubernetes
- authentication
- frontend
- cloud storage

unless explicitly requested.

These will be introduced in later stages.

---

## Future architecture

The eventual project may look approximately like:

```text
                    HTTP / gRPC API
                           |
                           v
                      Storage layer
                           |
              +------------+------------+
              |                         |
              v                         v
        PostgreSQL                 BlobStorage
        metadata                  filesystem
                                      |
                                      v
                                   blobs
```

The blob storage abstraction should therefore remain reasonably independent
from metadata storage.

---

## Development philosophy

This is a learning/portfolio project intended to demonstrate real backend
engineering skills.

Prefer implementations that demonstrate:

- concurrency
- correct error handling
- clean architecture
- testing
- filesystem I/O
- data integrity
- deterministic behavior
- maintainability

Do not overengineer the project prematurely.

When implementing a feature, first make the simplest correct version,
then improve it if there is a concrete reason.

Before making a large architectural change, explain the trade-offs.
````
