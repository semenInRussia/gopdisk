# GopherDisk

## Run Tests With PostgreSQL

Start a local PostgreSQL database for tests:

```bash
docker run --name gopherdisk-postgres \
  -e POSTGRES_USER=gopherdisk \
  -e POSTGRES_PASSWORD=gopherdisk \
  -e POSTGRES_DB=gopherdisk \
  -v gopherdisk-postgres-data:/var/lib/postgresql/data \
  -p 5432:5432 \
  -d postgres:16
```

If the container already exists:

```bash
docker start gopherdisk-postgres
```

Run migrations:

```bash
go run ./cmd/migrate up
```

Run all tests against the local database:

```bash
GOPHERDISK_DATABASE_TEST_URL='postgres://gopherdisk:gopherdisk@localhost:5432/gopherdisk?sslmode=disable' go test ./...
```

Run race tests:

```bash
GOPHERDISK_DATABASE_TEST_URL='postgres://gopherdisk:gopherdisk@localhost:5432/gopherdisk?sslmode=disable' go test -race ./...
```

## Run Project

Run the migrations, then start the HTTP API on `http://127.0.0.1:8080`:

```bash
go run ./cmd/migrate up
go run ./cmd/api
```

## Run Frontend

The simple React frontend lives in `frontend/`. Install its dependencies and
start the Vite development server:

```bash
cd frontend
npm install
npm run dev
```

Open `http://localhost:5173`. Create a production build with:

```bash
cd frontend
npm run build
```

The API sends registration messages through SMTP. Configure it with
`GOPHERDISK_SMTP_HOST`, `GOPHERDISK_SMTP_PORT`,
`GOPHERDISK_SMTP_USERNAME`, `GOPHERDISK_SMTP_PASSWORD`, and
`GOPHERDISK_SMTP_SENDER`. When host and port are unset, the current defaults
are Yandex Postbox at `postbox.cloud.yandex.net:587`. The equivalent `-smtp-*`
command-line flags remain available and take precedence over environment
variables.

For example, using Brevo:

```bash
export GOPHERDISK_SMTP_HOST='smtp-relay.brevo.com'
export GOPHERDISK_SMTP_PORT='587'
export GOPHERDISK_SMTP_USERNAME='your-brevo-smtp-login'
export GOPHERDISK_SMTP_PASSWORD='your-brevo-smtp-key'
export GOPHERDISK_SMTP_SENDER='GopherDisk <your-verified-sender@example.com>'

go run ./cmd/api
```

Do not commit real SMTP credentials to the repository.

Register a user with a name, email address, and password:

```bash
curl -X POST http://127.0.0.1:8080/v1/users \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Alice",
    "email": "alice@example.com",
    "password": "a-long-development-password"
  }'
```

Submit the token from the verification email to activate the account:

```bash
curl -X PUT http://127.0.0.1:8080/v1/users/activated \
  -H 'Content-Type: application/json' \
  -d '{"token":"TOKEN_FROM_EMAIL"}'
```

The activation response contains the user ID needed by the file endpoints.

## EXAMPLE: Upload an 80 MiB File Through the API

Suppose you have an 80 MiB video named `holiday.mov`. The API accepts parts of
up to 16 MiB, so you need to send the file in five parts. The commands below
assume that the API is running on `http://127.0.0.1:8080`.

First, check the size of your file:

```bash
wc -c < holiday.mov
```

For an 80 MiB file, the result is `83886080` bytes.

Register and activate the user as described above. The activation response
contains the new user ID:

```json
{
    "user": {
        "id": "baab6197-3e2d-4969-8492-247bbba900dc",
        "name": "Alice",
        "email": "alice@example.com",
        "activated": true
    }
}
```

Use that ID to start the upload. The `size` value is the size of the complete
file in bytes, not the size of one part:

```bash
curl -X POST http://127.0.0.1:8080/v1/files/upload \
  -H 'Content-Type: application/json' \
  -d '{
    "owner_id": "baab6197-3e2d-4969-8492-247bbba900dc",
    "name": "holiday.mov",
    "type": "video/quicktime",
    "size": 83886080
  }'
```

Copy the file ID from the response. In the commands below, the example file ID
is `32f11d17-b1f5-4fb0-94ca-614903665dff`.

Upload the five parts. Positions start at zero. Each `dd` command reads one
16 MiB part from the original file and sends it directly to the API:

```bash
dd if=holiday.mov bs=16777216 skip=0 count=1 2>/dev/null | \
  curl -X PUT http://127.0.0.1:8080/v1/uploads/32f11d17-b1f5-4fb0-94ca-614903665dff/parts/0 \
    -H 'Content-Type: application/octet-stream' --data-binary @-

dd if=holiday.mov bs=16777216 skip=1 count=1 2>/dev/null | \
  curl -X PUT http://127.0.0.1:8080/v1/uploads/32f11d17-b1f5-4fb0-94ca-614903665dff/parts/1 \
    -H 'Content-Type: application/octet-stream' --data-binary @-

dd if=holiday.mov bs=16777216 skip=2 count=1 2>/dev/null | \
  curl -X PUT http://127.0.0.1:8080/v1/uploads/32f11d17-b1f5-4fb0-94ca-614903665dff/parts/2 \
    -H 'Content-Type: application/octet-stream' --data-binary @-

dd if=holiday.mov bs=16777216 skip=3 count=1 2>/dev/null | \
  curl -X PUT http://127.0.0.1:8080/v1/uploads/32f11d17-b1f5-4fb0-94ca-614903665dff/parts/3 \
    -H 'Content-Type: application/octet-stream' --data-binary @-

dd if=holiday.mov bs=16777216 skip=4 count=1 2>/dev/null | \
  curl -X PUT http://127.0.0.1:8080/v1/uploads/32f11d17-b1f5-4fb0-94ca-614903665dff/parts/4 \
    -H 'Content-Type: application/octet-stream' --data-binary @-
```

After all five requests succeed, complete the upload:

```bash
curl -X POST \
  http://127.0.0.1:8080/v1/uploads/32f11d17-b1f5-4fb0-94ca-614903665dff/complete
```

The returned file should have the status `ready`. You can now download it:

```bash
curl -o downloaded-holiday.mov \
  http://127.0.0.1:8080/v1/files/32f11d17-b1f5-4fb0-94ca-614903665dff/content
```
