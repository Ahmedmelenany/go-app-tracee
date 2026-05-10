# go-app-tracee

A small REST API in Go for managing a list of `items`. Data is persisted to SQLite and the server shuts down gracefully on `SIGINT` / `SIGTERM`.

## Requirements

- Go **1.22** or newer (the router uses pattern-based `ServeMux` introduced in 1.22)

## Run

```sh
go mod tidy   # first time, fetches dependencies
go run .
```

The server listens on `:8080` and creates `app.db` in the working directory on first start.

## Endpoints

| Method | Path           | Description                  |
| ------ | -------------- | ---------------------------- |
| GET    | `/health`      | Health check                 |
| GET    | `/items`       | List all items               |
| POST   | `/items`       | Create an item               |
| GET    | `/items/{id}`  | Get one item by id           |
| PUT    | `/items/{id}`  | Update an item's name        |
| DELETE | `/items/{id}`  | Delete an item               |

Request/response bodies are JSON. An item has the shape `{"id": <int>, "name": <string>}`.

### Examples

```sh
# create
curl -X POST localhost:8080/items -d '{"name":"first"}'
# {"id":1,"name":"first"}

# list
curl localhost:8080/items

# get one
curl localhost:8080/items/1

# update
curl -X PUT localhost:8080/items/1 -d '{"name":"renamed"}'

# delete
curl -X DELETE localhost:8080/items/1
```

### Errors

- `400` — invalid JSON, missing/empty `name`, or non-numeric id
- `404` — item not found
- `500` — unexpected database error

## Tests

```sh
go test ./...
```

Tests run the real HTTP router against a temporary SQLite database (`t.TempDir()`), covering the full CRUD flow, validation errors, 404s, and invalid ids.

## Project layout

```
.
├── main.go         # store, router, server with graceful shutdown
├── main_test.go    # httptest-based API tests
├── go.mod
└── .gitignore
```

## Dependencies

- [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) — pure-Go SQLite driver (no cgo)
