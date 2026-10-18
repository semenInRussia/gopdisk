# GopherDisk HTTP API

This document describes the currently implemented API in `cmd/api`, including
its current limitations. All endpoints use the `/v1` prefix. User and file IDs
are UUID strings. This is a description of current behavior, not a list of
planned features.

## Conventions

### Requests and responses

JSON requests should use `Content-Type: application/json`. JSON responses use
`Content-Type: application/json`. The server does not currently enforce request
Content-Type or return `415` for an unsupported media type.

JSON bodies must contain a single JSON value and no unknown fields. The JSON
body limit is 1,048,576 bytes; exceeding it returns `400`, not `413`. Unless a
body is shown below, no request body is required. `204` responses have no body.

There is no pagination or pagination metadata. Query parameters such as
`limit`, `cursor`, and `cascade` are not implemented. `Idempotency-Key` is not
implemented.

### Authentication and access

These endpoints can be used without an authentication token:

- `GET /v1/healthcheck`
- `POST /v1/users`
- `PUT /v1/users/activated`
- `POST /v1/tokens/authentication`

All other documented endpoints require an authenticated, activated account:

```http
Authorization: Bearer <token>
```

Use the 26-character authentication token returned by the token endpoint, not
the activation token sent by email. Authentication middleware sets
`Vary: Authorization`. A supplied malformed, unknown, expired, or wrong-scope
token returns `401` even on public endpoints. No token on a protected endpoint
returns `401`; a valid token for an inactive user returns `403`.
Authentication-related `401` responses from middleware include
`WWW-Authenticate: Bearer`.

Only permissions management currently checks that the caller owns the file.
Other user, file, and upload handlers require activation but do not check
ownership or enforce stored file permissions. Upload creation still takes
`owner_id` from the request body.

### Errors

Ordinary errors use a string, without a machine-readable error code:

```json
{"error": "the requested resource could not be found"}
```

Validation errors use an object keyed by field name:

```json
{"error": {"codes": "must not contain duplicates"}}
```

| Status | Current meaning |
| --- | --- |
| `400` | Invalid JSON, unknown field, oversized JSON body, invalid path parameter, or empty upload part. |
| `401` | Missing authentication on a protected route, invalid token, or invalid login credentials. |
| `403` | Inactive account, or non-owner accessing permissions management. |
| `404` | Missing resource, unavailable file/upload state, or unknown route. |
| `405` | Unsupported method for a registered path. |
| `409` | Upload completion fails part or size validation. |
| `422` | Invalid fields, duplicate registration email, or invalid activation token. |
| `500` | Unexpected database, filesystem, or server error. |

Invalid UUID path parameters return `400` after authentication and any preceding
access checks. Endpoint-specific exceptions are documented below.

### Shared response objects

A `user` object includes `id`, `name`, `email`, and `activated`. Passwords and
password hashes are never returned:

```json
{
    "user": {
        "id": "baab6197-3e2d-4969-8492-247bbba900dc",
        "name": "Kotik",
        "email": "kotik@example.com",
        "activated": true
    }
}
```

A logical `file` object contains metadata, not blob paths or contents:

```json
{
    "file": {
        "id": "32f11d17-b1f5-4fb0-94ca-614903665dff",
        "name": "movie.mov",
        "type": "video/quicktime",
        "owner_id": "baab6197-3e2d-4969-8492-247bbba900dc",
        "status": "ready",
        "size": 104857600
    }
}
```

`size` is the logical size in bytes; status is `uploading` or `ready`.
Collection responses use arrays, including `[]` when empty.

## Endpoint summary

| Method | Path | Success response |
| --- | --- | --- |
| `GET` | `/v1/healthcheck` | `203`, health information |
| `POST` | `/v1/users` | `202`, registration message |
| `PUT` | `/v1/users/activated` | `200`, `user` |
| `POST` | `/v1/tokens/authentication` | `201`, `authentication_token` |
| `GET` | `/v1/users/:id` | `200`, `user` |
| `DELETE` | `/v1/users/:id` | `204` |
| `GET` | `/v1/users/:id/files` | `200`, `files` |
| `GET` | `/v1/users/:id/uploads` | `200`, `files` |
| `GET` | `/v1/files/:id` | `200`, `file` |
| `PATCH` | `/v1/files/:id` | `200`, `file` |
| `DELETE` | `/v1/files/:id` | `204` |
| `GET` | `/v1/files/:id/content` | `200`, raw file bytes |
| `HEAD` | `/v1/files/:id/content` | `200`, content headers only |
| `POST` | `/v1/files/upload` | `200`, `file` in `uploading` status |
| `GET` | `/v1/uploads/:file_id` | `200`, `upload` |
| `PUT` | `/v1/uploads/:file_id/parts/:position` | `201` or `200`, `part` |
| `DELETE` | `/v1/uploads/:file_id/parts/:position` | `204` |
| `POST` | `/v1/uploads/:file_id/complete` | `200`, `file` |
| `DELETE` | `/v1/uploads/:file_id` | `204` |
| `GET` | `/v1/files/:id/permissions` | `200`, `permissions` |
| `PUT` | `/v1/files/:id/permissions/:user_id` | `204` |
| `DELETE` | `/v1/files/:id/permissions/:user_id` | `204` |

## Service

### GET /v1/healthcheck

No body. Returns `203 Non-Authoritative Information` (the current handler's
status, not `200`):

```json
{
    "status": "available",
    "version": "1.0.0",
    "time": "2026-10-18T12:00:00Z"
}
```

This reports the running service; it does not probe the database or filesystem.

## Registration, activation, and tokens

### POST /v1/users

Request:

```json
{
    "name": "Kotik",
    "email": "kotik@example.com",
    "password": "long-enough-password"
}
```

Name must be nonblank and at most 500 characters. Email is trimmed and
lowercased, must match the email validator, and is limited to 500 characters.
Password must be 8–72 bytes. Invalid fields and duplicate email return `422`.

Returns `202 Accepted`, with no `Location` header or user object:

```json
{
    "message": "an email will be sent to you containing activation instructions"
}
```

The account starts with `activated: false`. An activation token valid for three
days is sent by background email; the response does not guarantee delivery.

### PUT /v1/users/activated

Request:

```json
{"token": "ABCDEFGHIJKLMNOPQRSTUVWXYZ"}
```

The token must be 26 bytes long and must be an unexpired activation token.
Returns `200 OK` with the shared `user` envelope and `activated: true`.
Missing, malformed-length, unknown, expired, or consumed tokens return `422`.
Successful activation consumes the user's activation tokens.

### POST /v1/tokens/authentication

Request:

```json
{
    "email": "kotik@example.com",
    "password": "long-enough-password"
}
```

Returns `201 Created`:

```json
{
    "authentication_token": {
        "token": "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
        "expiry": "2026-10-19T12:00:00Z"
    }
}
```

The token is valid for 24 hours. Invalid email format returns `422`.
Unknown email, incorrect password, or an inactive account returns `401` with
`{"error":"invalid authentication credentials"}`. This login error does not
set `WWW-Authenticate`.

## Users

### GET /v1/users/:id

No body. Returns `200 OK` with the shared `user` envelope, or `404` for a
nonexistent user.

### DELETE /v1/users/:id

No body. Returns `204 No Content`, or `404` for a nonexistent user. There is no
cascading-delete option for owned files. If the user still owns files or
uploads, the database foreign key prevents deletion; the current handler
reports this as `500`, not `409`.

### GET /v1/users/:id/files

No body. Returns `200 OK` with `{"files":[...]}`. Items use the shared file
shape, have status `ready`, and are ordered by name and ID. An empty collection
is `{"files":[]}`. There is no pagination metadata. A nonexistent user
currently returns `400`, not `404`.

### GET /v1/users/:id/uploads

No body. Returns `200 OK` with `{"files":[...]}`, using the shared file shape,
status `uploading`, ordered by name and ID. It does not return an `uploads`
envelope, part counts, parts, or received sizes. For progress details, use
`GET /v1/uploads/:file_id`. A nonexistent user currently returns `400`.

## Files

### GET /v1/files/:id

No body. Returns `200 OK` with the shared `file` envelope. Only completed files
are returned; an unfinished or nonexistent file returns `404`.

### PATCH /v1/files/:id

Request:

```json
{"name": "holiday.mov"}
```

Returns `200 OK` with the updated `file` envelope. Renaming changes metadata
only. An unfinished or nonexistent file returns `404`. Name must be nonblank
and at most 500 characters. The handler currently reuses the user-name
validator, so name validation errors use the key `user`, not `name`.

### DELETE /v1/files/:id

No body. Returns `204 No Content`, or `404` if the file does not exist.
This handler can delete either a completed file or an unfinished upload.
Deletion removes metadata and associated permissions. Physical blobs remain
eligible for reference-aware garbage collection.

### GET /v1/files/:id/content

No body. Returns `200 OK` with raw file bytes reconstructed from blobs in
position order. An unfinished or nonexistent file returns `404`.

The handler sets these content headers:

```http
Content-Type: video/quicktime
Content-Length: 104857600
```

Content-Type is the stored file `type`. The handler does not set
`Content-Disposition` or `Accept-Ranges`. Byte ranges are not implemented:
`Range` is ignored and the full file is returned, not `206` or `416`.
Errors after streaming begins are logged and may leave an incomplete download;
they cannot be replaced with a JSON error response.

### HEAD /v1/files/:id/content

No body. Returns `200 OK` with the same content headers as a full download,
without reading blobs or returning file contents. The content handler returns
`400`, `404`, or `500` without a body on parameter, lookup, or database errors.
Authentication middleware runs first and may reject the request before this
handler is reached.

## Resumable uploads

### POST /v1/files/upload

Request:

```json
{
    "owner_id": "baab6197-3e2d-4969-8492-247bbba900dc",
    "name": "movie.mov",
    "type": "video/quicktime",
    "size": 104857600
}
```

Owner ID must be a UUID referring to an existing user. Name must be nonblank
and at most 500 characters; type must be nonblank and at most 255 characters.
Size must be nonnegative; an omitted size currently defaults to zero. Invalid
fields return `422`; a nonexistent owner currently produces `500`.

Returns `200 OK` with a `file` envelope, not `201` or an `upload` envelope.
The shared file shape is used with `status: "uploading"`. No `Location` header
is set. Completed-file reads will not expose the file until completion.

There is no HTTP endpoint for replacing an existing file's contents:
`POST /v1/files/:id/upload` is not registered.

### GET /v1/uploads/:file_id

No body. Returns `200 OK`:

```json
{
    "upload": {
        "file_id": "32f11d17-b1f5-4fb0-94ca-614903665dff",
        "owner_id": "baab6197-3e2d-4969-8492-247bbba900dc",
        "name": "movie.mov",
        "type": "video/quicktime",
        "status": "uploading",
        "size": 104857600,
        "received_size": 33554432,
        "parts": [
            {"position": 0, "size": 16777216},
            {"position": 1, "size": 16777216}
        ]
    }
}
```

Parts are ordered by position. An upload without parts has `parts: []` and
`received_size: 0`. A completed, canceled, or nonexistent upload returns `404`.

### PUT /v1/uploads/:file_id/parts/:position

Request body is raw part bytes, conventionally sent as
`Content-Type: application/octet-stream`. Position is a nonnegative, zero-based
integer. Parts may be uploaded concurrently and out of order.

At most `BlobsMaxSize` bytes are read from each request, by default 16 MiB.
Additional bytes are not stored; send them as later parts. A new position
returns `201 Created`; replacing an existing position normally returns `200 OK`:

```json
{
    "part": {
        "position": 0,
        "size": 16777216
    }
}
```

Empty parts and negative positions return `400`. Completed, canceled, or
nonexistent uploads return `404`. Retrying a position replaces its contents
without renumbering other parts. The status distinction uses a lookup before
the write, so it is not a concurrency guarantee for simultaneous requests to
the same position.

### DELETE /v1/uploads/:file_id/parts/:position

No body. Returns `204 No Content`. Negative positions return `400`. A missing
part, or a completed, canceled, or nonexistent upload, returns `404`.
Removing a part does not renumber later positions.

### POST /v1/uploads/:file_id/complete

No body. Completion requires contiguous positions starting at zero, exactly
`BlobsMaxSize` bytes in every non-final part, at most `BlobsMaxSize` bytes in the
final part, and a total size matching the declared size. An empty file may be
completed with no parts when its declared size is zero.

Returns `200 OK` with the shared `file` envelope in `ready` status. Failed
validation returns `409` with a string error describing the mismatch; the
upload remains editable. A nonexistent or canceled upload returns `404`.
Repeating completion for an already completed file returns that file with
`200 OK`.

### DELETE /v1/uploads/:file_id

No body. Returns `204 No Content`. Canceling removes unfinished metadata and
blob associations; unreferenced physical blobs are eligible for cleanup.
A completed, already canceled, or nonexistent upload returns `404`.

## File permissions

These endpoints require an activated account that owns the completed file.
`:id` is the file UUID; `:user_id` identifies the user whose permissions are
being changed. Ownership is checked using `files.owner_id`.

Allowed codes are `files:read`, `files:write`, and `files:delete`. They are
independent stored permissions; no implication between codes is implemented.
These endpoints only manage records. Codes are not yet enforced on file or
upload operations. The owner's access to permissions management is based on
ownership, not these rows; changing rows does not change `owner_id`.

Common failures:

- `401`: missing or invalid authentication token.
- `403`: inactive account or caller is not the file owner.
- `400`: invalid UUID path parameter.
- `404`: file does not exist or is still uploading.
- `500`: unexpected database error.

Authentication, file lookup, and ownership checks precede body validation and
target-user validation, so an earlier error can take precedence.

### GET /v1/files/:id/permissions

Input: file UUID in the path, Bearer token in `Authorization`, no request body.

Returns `200 OK` with one row per `(file_id, user_id, code)`, ordered by user ID
and code:

```json
{
    "permissions": [
        {
            "file_id": "32f11d17-b1f5-4fb0-94ca-614903665dff",
            "user_id": "593fc45d-8640-4e4a-a488-aa753b27895c",
            "code": "files:read"
        },
        {
            "file_id": "32f11d17-b1f5-4fb0-94ca-614903665dff",
            "user_id": "593fc45d-8640-4e4a-a488-aa753b27895c",
            "code": "files:write"
        }
    ]
}
```

If no explicit permissions exist, the response is `{"permissions":[]}`.
Implicit owner access is not added to this list. There is no pagination.

### PUT /v1/files/:id/permissions/:user_id

Input: file and target-user UUIDs in the path, Bearer token in `Authorization`,
and JSON body:

```json
{
    "codes": ["files:read", "files:write"]
}
```

Replaces all explicit permissions for this user and file in one transaction.
Other users' rows are unaffected. The target user must exist. Repeating the
same request leaves the same set of permissions.

Returns `204 No Content` with an empty body. To revoke all explicit permissions,
send `{"codes":[]}`, which also returns `204`.

Additional failures:

- `400`: malformed JSON, wrong field type, unknown JSON field, or oversized body.
- `422`: missing or null `codes`, an unknown code, or duplicate codes.
- `404`: target user does not exist.

For example, a repeated code produces:

```http
HTTP/1.1 422 Unprocessable Entity
Content-Type: application/json
```

```json
{
    "error": {
        "codes": "must not contain duplicates"
    }
}
```

### DELETE /v1/files/:id/permissions/:user_id

Input: file and target-user UUIDs in the path, Bearer token in `Authorization`,
no request body.

Revokes all explicit permissions for this user on this file. Returns
`204 No Content` with an empty body, including when the user has no permission
rows or the target user does not exist. The file must still exist in `ready`
status and belong to the caller, even for repeated deletes.

Deleting a file or user also removes their permission rows through database
foreign keys with `ON DELETE CASCADE`.
