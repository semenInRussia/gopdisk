# GopherDisk Deep Code Review

## Executive Summary

The architecture is directionally sound, but the storage engine is not yet safe under concurrent uploads and garbage collection. The most serious issues can cause committed files to reference missing or corrupted blobs.

The logical-file/physical-blob separation is preserved well. Blob order is explicit in PostgreSQL, append operations serialize on the file row, metadata transitions from `uploading` to `ready`, reads preserve order, deduplication is modeled correctly, and cancellation/error channels are substantially better than the earlier implementation.

Before building a production-style HTTP layer, the blob lifecycle and filesystem safety issues below should be fixed.

## Findings

### Critical: Garbage collection races with uploads

Locations:

- `internal/data/storage.go:79-98`
- `internal/data/files.go:147-158`

`CollectGarbageBlobs` snapshots the hashes known to PostgreSQL and then deletes every filesystem blob not in that snapshot. Meanwhile, `AddData` writes physical blobs before registering them in PostgreSQL.

A valid interleaving is:

1. An upload writes a new physical blob.
2. Garbage collection queries PostgreSQL before `appendBlobs` runs.
3. Garbage collection sees the physical blob but not its database row and deletes it.
4. The upload inserts the blob metadata and commits successfully.
5. The logical file becomes `ready`, but reading it fails because its physical blob is gone.

This violates the core data-integrity contract.

A process-local mutex could address a single-process deployment. For multi-process safety, use a database-coordinated lock, a staging area excluded from collection, or an explicit blob lifecycle/state that garbage collection respects.

### High: Content-addressed blobs are overwritten non-atomically

Location: `internal/blobs/blobs.go:195-198`

`writeFile` uses `os.WriteFile` directly on the final hash path. This truncates an existing file before rewriting it.

Because identical chunks share a hash, uploading duplicate content can rewrite a blob already used by ready files. A concurrent reader can observe the file between truncation and completion and receive `io.ErrUnexpectedEOF`. A process crash can leave a permanently truncated shared blob.

Write to a temporary file in the same directory, flush and close it, and then atomically install it. Existing valid hash files should not be truncated in place.

### High: `AddBlob` permits path traversal and untrusted metadata

Locations:

- `internal/data/files.go:173-175`
- `internal/blobs/blobs.go:168-181`
- `internal/blobs/blobs.go:190-202`

`AddBlob` accepts arbitrary `Blob` values without checking:

- that the hash is a valid SHA-256 hexadecimal string;
- that the physical blob exists;
- that its size is nonnegative and no greater than `BlobsMaxSize`;
- that its contents match the supplied hash.

The hash later becomes part of a filesystem path. A value such as `../secret` can therefore make a committed logical file read outside the blob directory. An arbitrary size can also cause an excessive allocation in `ReadBlob`.

This is especially dangerous if `AddBlob` is eventually exposed through an HTTP chunk-upload handler. Validate the complete blob identity at the blob-storage boundary rather than trusting caller-provided metadata.

### Medium: `Rename` can report success after concurrent deletion

Location: `internal/data/files.go:227-243`

`Rename` first loads the file and then performs a separate `UPDATE`, but it never checks `RowsAffected`.

If another goroutine deletes the file between those operations, the update affects zero rows and `Rename` still returns a successfully renamed `File` value even though the database row no longer exists.

Use one `UPDATE ... RETURNING` statement. `Delete` has a similar check-then-delete race at `internal/data/files.go:215-225`, allowing two concurrent deletions to both report success.

### Medium: Failed-upload cleanup is best-effort and silently discarded

Locations:

- `internal/data/files.go:112-127`
- `internal/data/files.go:129-144`

The upload helpers ignore every cleanup error:

```go
_ = s.Delete(context.WithoutCancel(ctx), file.ID)
```

If metadata insertion fails because the database is unavailable, cleanup is likely to fail for the same reason. The API returns only the original error while leaving an `uploading` file record behind.

`context.WithoutCancel` also removes the original deadline, so cleanup can wait indefinitely. Return or join the cleanup error, apply a bounded cleanup context, and provide a strategy for expiring abandoned uploads.

### Medium: Blob corruption is not detected during reads

Location: `internal/blobs/blobs.go:168-181`

`ReadBlob` reads the recorded number of bytes but never recomputes the SHA-256 hash. Same-size corruption is returned as valid content. It also does not detect trailing data beyond the recorded size.

Because the hash is the blob identity, verify it during reads or provide a separate integrity-scrubbing operation.

### Medium: Migration execution can silently do nothing

Locations:

- `cmd/migrate/main.go:41-54`
- `cmd/migrate/main.go:94-101`

`migrationFiles` returns an empty list when the current working directory does not contain `migrations/`. `runUp` then exits successfully.

A packaged binary started from another directory can therefore claim migration success without creating any tables. Return an error when no migrations are found, or embed migrations in the binary.

### Medium: PostgreSQL behavior is effectively untested by default

Locations:

- `internal/data/storage_test.go:38-55`
- `internal/data/storage_test.go:797-819`

The ordinary test, race-test, and vet commands pass. However, the storage tests skip unless external PostgreSQL URLs are configured. During this review, every `internal/data` test except the standalone error-mapping test was skipped.

Consequences include:

- the race run does not exercise uploads, commits, deletion, or garbage collection;
- there are no concurrent upload/garbage-collection tests;
- database tests share persistent state rather than isolating each test;
- the separate garbage-collection database requirement is easy to omit in CI;
- content-update behavior described by the project contract is neither implemented nor tested.

CI should provision PostgreSQL, apply migrations, and fail if the integration suite is skipped. Tests should use isolated schemas, transactions, or reliable cleanup.

## Lower-Priority Issues

### PostgreSQL `BIGINT` values are scanned into `int`

Location: `internal/data/files.go:186-205`

`Commit` scans declared and stored sizes into `int`, undoing the otherwise correct `int64` size model. This can overflow on 32-bit targets. Use `int64` consistently.

### `BlobsMaxSize` is mutable global state

Location: `internal/blobs/blobs.go:18`

Concurrent mutation can introduce data races and inconsistent chunk boundaries. Prefer a constant or per-storage immutable configuration.

### Rejected `AddData` calls create filesystem garbage

Location: `internal/data/files.go:147-158`

`AddData` stores the physical blobs before establishing that the target file exists and is still uploading. Calling it for a missing or committed file writes orphaned blobs before returning an error.

### File permissions are broad for private data

Locations:

- `internal/blobs/blobs.go:34`
- `internal/blobs/blobs.go:98`

Blob directories and files use `0755` and `0644`. For private user data, `0700` and `0600` are safer defaults.

### The large-file test fixture is unnecessarily expensive

Location: `internal/data/testdata/movie.mov`

The repository contains an 80 MB test fixture. The test reads it before discovering that PostgreSQL tests are disabled. Large-file behavior can be tested with generated deterministic data without carrying a large binary in the repository.

## Verification

The following commands were run:

```bash
env GOCACHE=/private/tmp/gopherdisk-review-cache go test ./...
env GOCACHE=/private/tmp/gopherdisk-review-cache go vet ./...
env GOCACHE=/private/tmp/gopherdisk-review-race-cache go test -race ./...
env GOCACHE=/private/tmp/gopherdisk-review-cache go test -v ./internal/data
```

`go test`, `go vet`, and `go test -race` passed. The verbose run confirmed that PostgreSQL-dependent storage tests were skipped because `GOPHERDISK_DATABASE_TEST_URL` and `GOPHERDISK_GARBAGE_COLLECTION_TEST_URL` were not configured.

## Recommended Order of Work

1. Make blob installation atomic.
2. Coordinate garbage collection with uploads.
3. Validate or remove the arbitrary `AddBlob` boundary.
4. Replace check-then-write operations with atomic SQL using `RETURNING`.
5. Make failed-upload cleanup observable and bounded.
6. Run isolated PostgreSQL tests in CI, including concurrent upload, garbage-collection, and read/write tests.
7. Add integrity verification and content-update behavior.

## Readiness Assessment

The project is suitable for continued storage-engine development, but the current implementation should not be placed behind a production-style web API yet. A thin experimental API is possible only if garbage collection is disabled during uploads and the low-level `AddBlob` operation is not exposed to untrusted input.
