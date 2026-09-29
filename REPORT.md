# GopherDisk Web-Server Readiness Report

Date: 2026-09-27

## Conclusion

GopherDisk is **not yet ready to serve as a dependable web-server backend**.
The architecture is promising, but the storage layer should go through one
hardening milestone before HTTP handlers are built directly on top of it.

The project is ready to continue *toward* a web server, including preliminary
server scaffolding, but its current storage API should not yet be treated as a
stable or production-safe boundary.

## Main blockers

### 1. PostgreSQL-backed tests were not executed

The following checks passed:

- `gofmt -d internal cmd`
- `go vet ./...`
- `go test -count=1 ./...`
- `go test -race -count=1 ./...`

However, every PostgreSQL-backed storage test was skipped because
`GOPHERDISK_DATABASE_TEST_URL` and
`GOPHERDISK_GARBAGE_COLLECTION_TEST_URL` were unset. The existing
`gopherdisk-postgres` Docker container was stopped during the review.

This means the core storage lifecycle and its race-enabled integration tests
were not actually verified during this assessment. The test setup currently
skips these tests in `internal/storage/storage_test.go` when the environment
variables are missing.

### 2. Read APIs hide database failures as missing resources

`GetFile` and `GetUser` return only pointers. Their internal loading helpers
discard database and context errors and return `nil`.

Consequences for an HTTP layer include:

- a database outage may be reported as HTTP 404 instead of HTTP 500;
- canceled requests may appear to be missing resources;
- callers cannot reliably distinguish absence from infrastructure failure.

These APIs should return `(*File, error)` and `(*User, error)`, with a sentinel
not-found error where appropriate.

### 3. Multipart upload finalization is vulnerable to races

`UploadPart` checks the file's uploading status before writing the blob, while
`FinishUpload` independently loads the file, checks its size, and changes its
status to ready.

With concurrent HTTP requests, a part can be replaced or associated while the
file is being finalized. Validation and mutation are not serialized in one
transaction.

The upload state transition should be protected transactionally, likely by
locking the logical file row while attaching parts and while finalizing it.
Tests should exercise concurrent `UploadPart` and `FinishUpload` calls.

### 4. Filesystem and database consistency is not crash-safe

Blob data is written directly to its final hash path before its metadata is
committed to PostgreSQL.

This creates several failure modes:

- a failed database operation can leave a physical file with no database row;
- the current garbage collector cannot discover files that have no database
  row;
- garbage collection can race with an upload that has written a file but has
  not yet associated it;
- a process crash during `os.WriteFile` can leave a partial final blob;
- readers can potentially observe a blob while it is being written.

Blob writes should use a temporary file followed by an atomic rename. The
project also needs a clear staging or filesystem-reconciliation strategy for
untracked physical blobs. Garbage collection and upload association must not be
able to delete or lose a blob that is becoming referenced.

### 5. Upload size validation is incomplete

Current issues include:

- negative file sizes are accepted by `PreUpload`;
- the database has no `CHECK (size >= 0)` constraint;
- multipart uploads validate part position but not the exact expected size of
  each part;
- `UploadReader` reads only the declared size and does not detect trailing
  input;
- file and blob sizes use `int`, while PostgreSQL stores `BIGINT`.

The storage contract should define exact size behavior before it is exposed
through HTTP. Sizes should preferably use `int64` across public APIs and
database scanning.

## Important improvements

These are important for the first useful web-server version, although they do
not all need to block initial scaffolding:

- `ListFiles` performs an N+1 query and loads every listed file's complete blob
  list. Add pagination and a metadata-only list query.
- Ownership is checked separately through `FileBelongsToUser`. This makes it
  easy for handlers to forget authorization or introduce a time-of-check versus
  time-of-use problem. Prefer owner-scoped storage operations.
- Blob reads trust metadata size and do not verify the stored SHA-256 hash.
- Cleanup errors after failed uploads are ignored.
- The Dockerfile runs the test suite rather than building and running an
  application server.
- The project does not yet have CI that guarantees PostgreSQL integration tests
  are executed rather than skipped.
- There are minor code-quality issues, including an unused package variable,
  a redundant maximum-part-size variable, and misspelled/non-idiomatic error
  messages.

## Existing strengths

The project already has a strong foundation:

- PostgreSQL metadata and physical blob storage are separated correctly.
- Logical files are not collapsed into physical filesystem files.
- Blob order is stored explicitly and reconstructed deterministically.
- `UploadReader` supports streaming uploads without buffering the whole logical
  file in memory.
- Updating contents preserves the old database association until the new blobs
  have been stored.
- Context cancellation and read-channel closure have received attention.
- UUID identifiers, upload status, migrations, content hashes, and
  reference-aware garbage collection are sensible architectural choices.
- Empty, exact-boundary, multi-part, missing-file, update, delete, and
  persistence behaviors have test coverage, although the database-backed tests
  must be run in a configured environment.

## Recommended development sequence

1. Change storage lookups to return explicit errors and add complete input-size
   validation.
2. Make multipart attachment and finalization transactionally safe.
3. Implement atomic blob writes and a coherent orphan/reconciliation strategy.
4. Run the complete PostgreSQL suite and race detector in an environment where
   skipped storage tests cause CI failure.
5. Add concurrency and failure-injection tests for upload finalization, updates,
   cleanup, and garbage collection.
6. Add a thin HTTP server with:
   - streamed request bodies and downloads;
   - request and file-size limits;
   - consistent domain-error-to-HTTP-status mapping;
   - owner-scoped file operations;
   - list pagination;
   - server timeouts and graceful shutdown;
   - health and readiness checks.

## Final assessment

GopherDisk is ready to continue toward web-server development, but it is not
ready for a web server to depend directly on the current storage contract.
Complete the storage-hardening work first, then keep the HTTP layer thin and
focused on transport concerns.
