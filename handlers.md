# GopherDisk HTTP API

GopherDisk provides a local HTTP API for managing users, logical files, and
resumable uploads. File metadata is stored in PostgreSQL. File contents are
split into ordered, content-addressed blobs on the local filesystem.

This document defines the production API contract for version `v1`.

## Conventions

### Base path

All endpoints use the `/v1` prefix.

### Content types

- JSON requests use `Content-Type: application/json`.
- File parts and downloads use `application/octet-stream` unless a file has a
  more specific media type.
- All JSON responses are UTF-8 encoded.

### IDs

User IDs and file IDs are UUID strings. Invalid UUIDs return `400 Bad Request`.

### Resource envelopes

Successful JSON responses wrap their primary resource:

```json
{
    "file": {
        "id": "32f11d17-b1f5-4fb0-94ca-614903665dff",
        "name": "movie.mov",
        "file_type": "video/quicktime",
        "owner_id": "baab6197-3e2d-4969-8492-247bbba900dc",
        "status": "ready",
        "size": 104857600
    }
}
```

Collections use the plural resource name and include pagination metadata:

```json
{
    "files": [],
    "pagination": {
        "next_cursor": null
    }
}
```

### Errors

Errors have a stable machine-readable code and a human-readable message:

```json
{
    "error": {
        "code": "file_not_found",
        "message": "file not found"
    }
}
```

Validation errors may include field details:

```json
{
    "error": {
        "code": "validation_failed",
        "message": "request validation failed",
        "fields": {
            "size": "must not be negative"
        }
    }
}
```

Common status codes:

| Status | Meaning                                                         |
| ------ | --------------------------------------------------------------- |
| `400`  | Malformed JSON, invalid UUID, cursor, range, or path parameter. |
| `404`  | The requested user, file, upload, or part does not exist.       |
| `409`  | The request conflicts with the resource's current state.        |
| `413`  | A request body or uploaded part exceeds the allowed size.       |
| `415`  | The request uses an unsupported content type.                   |
| `416`  | A download range cannot be satisfied.                           |
| `422`  | The request is well-formed but fails field validation.          |
| `500`  | An unexpected database, filesystem, or server error occurred.   |

### Pagination

Collection endpoints accept:

- `limit`: number of results, from `1` to `100`; default `25`;
- `cursor`: opaque cursor returned by the preceding response.

Clients must not parse or construct cursors.

### Idempotency

Part uploads are idempotent by file ID and position. Retrying the same `PUT`
replaces that position without changing the order of other parts.

Upload-creation requests may include an `Idempotency-Key` header. Repeating a
request with the same key and body returns the original upload. Reusing the key
with a different body returns `409 Conflict`.

## Endpoint summary

### Service

| Method | Path              | Purpose                                      |
| ------ | ----------------- | -------------------------------------------- |
| `GET`  | `/v1/healthcheck` | Read service health and version information. |

### Users

| Method   | Path                    | Purpose                             |
| -------- | ----------------------- | ----------------------------------- |
| `POST`   | `/v1/users`             | Create a user.                      |
| `GET`    | `/v1/users/:id`         | Read a user.                        |
| `DELETE` | `/v1/users/:id`         | Delete a user.                      |
| `GET`    | `/v1/users/:id/files`   | List the user's completed files.    |
| `GET`    | `/v1/users/:id/uploads` | List the user's unfinished uploads. |

### Files

| Method   | Path                    | Purpose                                  |
| -------- | ----------------------- | ---------------------------------------- |
| `GET`    | `/v1/files/:id`         | Read completed file metadata.            |
| `PATCH`  | `/v1/files/:id`         | Rename a completed file.                 |
| `DELETE` | `/v1/files/:id`         | Delete a logical file.                   |
| `GET`    | `/v1/files/:id/content` | Download file contents.                  |
| `HEAD`   | `/v1/files/:id/content` | Read download metadata without the body. |

### Uploads

| Method   | Path                                   | Purpose                              |
| -------- | -------------------------------------- | ------------------------------------ |
| `POST`   | `/v1/files/upload`                     | Start a new-file upload.             |
| `POST`   | `/v1/files/:id/upload`                 | Start an atomic content replacement. |
| `GET`    | `/v1/uploads/:file_id`                 | Read upload progress.                |
| `PUT`    | `/v1/uploads/:file_id/parts/:position` | Upload or replace a part.            |
| `DELETE` | `/v1/uploads/:file_id/parts/:position` | Remove an uploaded part.             |
| `POST`   | `/v1/uploads/:file_id/complete`        | Validate and complete an upload.     |
| `DELETE` | `/v1/uploads/:file_id`                 | Cancel an unfinished upload.         |

## Service

### Get health

```http
GET /v1/healthcheck
```

Returns `200 OK` when the service can accept requests.

```json
{
    "status": "available",
    "version": "1.0.0",
    "time": "2026-10-10T12:00:00Z"
}
```

## Users

### Create a user

```http
POST /v1/users
Content-Type: application/json
```

```json
{
    "name": "Kotik"
}
```

Returns `201 Created`, a `Location` header, and the created user:

```json
{
    "user": {
        "id": "baab6197-3e2d-4969-8492-247bbba900dc",
        "name": "Kotik"
    }
}
```

### Get a user

```http
GET /v1/users/:id
```

Returns `200 OK` with a `user` envelope or `404 Not Found`.

### Delete a user

```http
DELETE /v1/users/:id
```

Returns `204 No Content`. By default, deletion returns `409 Conflict` if the
user still owns files or unfinished uploads.

To delete the user and all owned logical files and uploads:

```http
DELETE /v1/users/:id?cascade=true
```

Physical blobs are removed only when no other logical file references them.

### List a user's files

```http
GET /v1/users/:id/files?limit=25&cursor=opaque-cursor
```

Returns only files in `ready` status, ordered by name and ID.

```json
{
    "files": [
        {
            "id": "32f11d17-b1f5-4fb0-94ca-614903665dff",
            "name": "movie.mov",
            "file_type": "video/quicktime",
            "owner_id": "baab6197-3e2d-4969-8492-247bbba900dc",
            "status": "ready",
            "size": 104857600
        }
    ],
    "pagination": {
        "next_cursor": null
    }
}
```

### List a user's uploads

```http
GET /v1/users/:id/uploads?limit=25&cursor=opaque-cursor
```

Returns unfinished uploads in `uploading` status. Each item includes the
declared size, received size, and uploaded part count.

## Files

### Get file metadata

```http
GET /v1/files/:id
```

Returns metadata only for a completed file. An unfinished or nonexistent file
returns `404 Not Found`; upload progress is available from the upload endpoint.

### Rename a file

```http
PATCH /v1/files/:id
Content-Type: application/json
```

```json
{
    "name": "holiday.mov"
}
```

Returns `200 OK` with the updated `file` envelope. Renaming changes metadata
only and does not rewrite blobs.

### Delete a file

```http
DELETE /v1/files/:id
```

Returns `204 No Content`. The logical file becomes unavailable immediately.
Unreferenced physical blobs are eligible for cleanup; shared blobs remain.

### Download file contents

```http
GET /v1/files/:id/content
```

Returns the logical file reconstructed from its blobs in position order.

Response headers include:

```http
Content-Type: video/quicktime
Content-Length: 104857600
Content-Disposition: attachment; filename="movie.mov"
Accept-Ranges: bytes
```

Clients may resume downloads with a single byte range:

```http
GET /v1/files/:id/content
Range: bytes=16777216-
```

A valid range returns `206 Partial Content` and `Content-Range`. An invalid or
unsatisfiable range returns `416 Range Not Satisfiable`.

`HEAD /v1/files/:id/content` returns the same download headers without a body.
The server streams data and stops reading blobs when the client disconnects.

## Resumable uploads

### Start a new-file upload

```http
POST /v1/files/upload
Content-Type: application/json
Idempotency-Key: client-generated-key
```

```json
{
    "owner_id": "baab6197-3e2d-4969-8492-247bbba900dc",
    "name": "movie.mov",
    "file_type": "video/quicktime",
    "size": 104857600
}
```

`size` is the total logical file size in bytes and must not be negative. The
owner must exist.

Returns `201 Created`, a `Location` header pointing to the upload status
endpoint, and an upload resource:

```json
{
    "upload": {
        "file_id": "32f11d17-b1f5-4fb0-94ca-614903665dff",
        "owner_id": "baab6197-3e2d-4969-8492-247bbba900dc",
        "name": "movie.mov",
        "file_type": "video/quicktime",
        "status": "uploading",
        "size": 104857600,
        "received_size": 0,
        "parts": []
    }
}
```

The file is not visible through completed-file endpoints until completion.

### Start a content replacement

```http
POST /v1/files/:id/upload
Content-Type: application/json
```

```json
{
    "size": 125829120,
    "file_type": "video/quicktime"
}
```

Returns `201 Created` with an upload resource. The existing file and its old
contents remain readable until the replacement upload completes. Completion
atomically swaps the blob list; a failed or canceled replacement leaves the
existing file unchanged. Only one active replacement is allowed per file;
another attempt returns `409 Conflict`.

### Get upload progress

```http
GET /v1/uploads/:file_id
```

```json
{
    "upload": {
        "file_id": "32f11d17-b1f5-4fb0-94ca-614903665dff",
        "owner_id": "baab6197-3e2d-4969-8492-247bbba900dc",
        "name": "movie.mov",
        "file_type": "video/quicktime",
        "status": "uploading",
        "size": 104857600,
        "received_size": 33554432,
        "parts": [
            {
                "position": 0,
                "size": 16777216
            },
            {
                "position": 1,
                "size": 16777216
            }
        ]
    }
}
```

Parts are always returned in ascending position order.

### Upload or replace a part

```http
PUT /v1/uploads/:file_id/parts/:position
Content-Type: application/octet-stream
Content-Length: 16777216
```

Positions are zero-based. Parts may be sent concurrently and out of order. A
part must not exceed the server's `BlobsMaxSize` limit, currently 16 MiB.

- A new position returns `201 Created`.
- Replacing an existing position returns `200 OK`.
- Uploading to a completed or canceled upload returns `409 Conflict`.

```json
{
    "part": {
        "position": 0,
        "size": 16777216
    }
}
```

The replacement becomes visible atomically; readers never observe a partially
replaced part.

### Remove a part

```http
DELETE /v1/uploads/:file_id/parts/:position
```

Returns `204 No Content`. Removing a nonexistent position returns `404 Not
Found`. Removing a part does not renumber later positions.

### Complete an upload

```http
POST /v1/uploads/:file_id/complete
```

Completion succeeds only when:

- the upload is still in `uploading` status;
- part positions start at zero and are contiguous;
- every non-final part is exactly `BlobsMaxSize`;
- the final part is no larger than `BlobsMaxSize`;
- the sum of part sizes equals the declared logical file size.

An empty file may be completed without parts when its declared size is zero.

Returns `200 OK` with the completed `file` envelope. Validation failures return
`409 Conflict` with details about missing positions or a size mismatch. The
upload remains editable after a failed completion attempt.

Completion is idempotent: repeating it for the same successfully completed
upload returns the completed file.

### Cancel an upload

```http
DELETE /v1/uploads/:file_id
```

Returns `204 No Content`. Canceling removes unfinished file metadata and makes
unreferenced uploaded blobs eligible for cleanup. It never changes an existing
file during a canceled content replacement.

## Data integrity and concurrency guarantees

- A logical file is separate from its physical blobs.
- Blob positions define file order and are preserved during concurrent upload.
- Part replacement and upload completion are transactional at the metadata
  layer.
- A file becomes readable only after successful completion.
- Failed content replacement never changes the current readable blob list.
- Deletion never removes a blob still referenced by another logical file.
- Request cancellation stops ongoing uploads and downloads without leaking
  goroutines.

## Authentication

The `v1` API does not implement authentication. It is intended for trusted
local use. Callers provide `owner_id` explicitly, and the server validates
resource ownership relationships. Authentication and authorization can be
added at the transport boundary in a later API version without exposing blob
storage internals.
