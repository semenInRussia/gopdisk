Yes—resumable part uploads fit this project well. I’d separate upload management from completed-file routes.

### Upload routes

| Method   | Path                                                  | Purpose                                                      |
| -------- | ----------------------------------------------------- | ------------------------------------------------------------ |
| `POST`   | `/v1/users/:user_id/uploads`                          | Start an upload; accept `name`, `filetype`, and total `size` |
| `GET`    | `/v1/users/:user_id/uploads/:file_id`                 | Get upload status and uploaded parts                         |
| `PUT`    | `/v1/users/:user_id/uploads/:file_id/parts/:position` | Upload or replace one part                                   |
| `POST`   | `/v1/users/:user_id/uploads/:file_id/complete`        | Validate all parts and mark the file ready                   |
| `DELETE` | `/v1/users/:user_id/uploads/:file_id`                 | Cancel an unfinished upload                                  |

Use zero-based positions:

```text
parts/0
parts/1
parts/2
```

Make `PUT` idempotent: uploading the same position again should replace that part. Each part should be no larger than `BlobsMaxSize`.

### Completed-file routes

| Method   | Path                                        | Purpose                         |
| -------- | ------------------------------------------- | ------------------------------- |
| `GET`    | `/v1/users/:user_id/files`                  | List completed files            |
| `GET`    | `/v1/users/:user_id/files/:file_id`         | Get file metadata               |
| `GET`    | `/v1/users/:user_id/files/:file_id/content` | Download the reconstructed file |
| `PATCH`  | `/v1/users/:user_id/files/:file_id`         | Rename the file                 |
| `DELETE` | `/v1/users/:user_id/files/:file_id`         | Delete the file                 |

### User and service routes

| Method   | Path                 | Purpose        |
| -------- | -------------------- | -------------- |
| `GET`    | `/v1/healthcheck`    | Service health |
| `POST`   | `/v1/users`          | Create user    |
| `GET`    | `/v1/users/:user_id` | Get user       |
| `DELETE` | `/v1/users/:user_id` | Delete user    |

Important: the current `AddData` method only appends blobs. It is not sufficient for concurrent part uploads because arrival order may differ from part order. Add something like `PutPart(fileID, position, data)` at the storage layer, then make completion verify:

- Positions are contiguous.
- Total blob size equals the declared file size.
- Every non-final part has the expected part size.
- The file is still in `uploading` status.
