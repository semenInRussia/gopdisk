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

The project currently exposes the migration command:

```bash
go run ./cmd/migrate up
```
