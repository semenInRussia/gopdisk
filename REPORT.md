# GopherDisk Code Review

## Question

Is the project ready to implement a web server?

## Short Answer

Yes, the project is ready to start a thin experimental web server over the storage layer.

It is not yet ready for a production-style HTTP API without a few storage contract and testing improvements. The core shape is good: context-aware methods, PostgreSQL metadata, filesystem-backed blobs, ordered reads, migrations, and basic race-test cleanliness are already in place.

## Findings

### 1. Chunk upload validation is too loose

Location: `internal/storage/files.go`

`UploadFilePart` validates only the part index. It does not validate that a part has the expected size for its position.

`FinishUploadFile` checks only the total uploaded byte count. This means an upload can use incorrect chunk boundaries while still becoming `ready` if the total size matches.

For a web chunk-upload API, each part should be validated against the declared file size:

- all non-final chunks should be exactly `blobs.BlobsMaxSize`
- the final chunk should match the remaining size
- empty files should have a clear behavior

### 2. File listing has an N+1 query pattern

Location: `internal/storage/files.go`

`ListFiles` first queries file IDs and then calls `loadFileResult` for every file. This is acceptable for small tests, but a web `GET /files` endpoint will hit this quickly.

Prefer one query for file metadata, and load blob lists only when needed or with a joined/aggregated query.

### 3. PostgreSQL-backed tests are easy to skip accidentally

Location: `internal/storage/storage_test.go`

Most storage tests skip unless `GOPHERDISK_DATABASE_TEST_URL` is set. A plain `go test ./...` can pass while important PostgreSQL-backed behavior was not exercised.

Before adding a serious web layer, CI should run these tests against PostgreSQL. Test isolation should also be tightened with one of:

- per-test cleanup
- per-test schemas
- transactions
- database truncation between tests

### 4. Domain error mapping needs to be stronger for HTTP

Location: `internal/storage/files.go`

The storage layer maps some size constraint violations to `ErrStorageInvalidSize`, but many database errors still leak upward directly.

A web server will need stable errors so handlers can reliably map storage outcomes to HTTP responses such as:

- `400 Bad Request`
- `404 Not Found`
- `409 Conflict`
- `500 Internal Server Error`

Examples worth normalizing include invalid IDs, foreign-key violations, duplicate constraints, and missing users/files.

## Verification

The following commands were run with a temporary Go build cache:

```bash
env GOCACHE=/private/tmp/gopherdisk-go-cache go test ./...
env GOCACHE=/private/tmp/gopherdisk-go-cache go vet ./...
env GOCACHE=/private/tmp/gopherdisk-go-cache go test -race ./...
```

All commands passed.

Important caveat: PostgreSQL-dependent storage tests skip unless the required environment variables are configured.

## Recommendation

Start the web server as a thin adapter around the current storage layer, but keep the first HTTP implementation small.

Recommended first endpoints:

- create user
- upload full file from request body
- list files for a user
- read/download file
- rename file
- delete file

Before exposing multipart/chunk upload publicly, fix chunk-size validation and add integration tests for out-of-order, duplicate, missing, and wrong-sized parts.
