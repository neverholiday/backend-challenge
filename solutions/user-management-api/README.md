# User Management API

[![CI](https://github.com/neverholiday/backend-challenge/actions/workflows/ci.yml/badge.svg?branch=feat/user_management_api_v2)](https://github.com/neverholiday/backend-challenge/actions/workflows/ci.yml?query=branch%3Afeat%2Fuser_management_api_v2)

RESTful user management API in Go, backed by MongoDB, with JWT (HS256) authentication.
Built with hexagonal architecture (ports & adapters). Also exposes a gRPC service for
`CreateUser`/`GetUser`.

## Status

| Layer | Status |
| --- | --- |
| Domain (`internal/domain`) | Done - `User` entity, `UserRepository`/`TokenService`/`PasswordHasher` ports, sentinel errors |
| Application (use cases) | Done - `RegisterUser`, `AuthenticateUser`, `GetUser`, `ListUsers`, `UpdateUser`, `DeleteUser` |
| Adapters - MongoDB | Done - `UserRepository`, unique email index |
| Adapters - bcrypt | Done - `PasswordHasher` |
| Adapters - JWT | Done - HS256 `TokenService` |
| Adapters - HTTP | Done - echo handlers, routes, logging + auth middleware, validation |
| Adapters - gRPC | Done - `CreateUser`/`GetUser` over `UserService`, code generated from `api/proto/user/v1/user.proto` |
| Concurrency task | Done - `UserCountReporter` logs the total user count every 10s |
| Docker / docker-compose | Done - multi-stage `Dockerfile`, `docker-compose.yml` (api + mongo), healthchecks |
| Graceful shutdown | Done - `signal.NotifyContext`, HTTP `Shutdown`, gRPC `GracefulStop`, Mongo `Disconnect` |

## Architecture

Hexagonal (ports & adapters):

```
api/proto/user/v1/  UserService proto definition
cmd/
  api/              HTTP + gRPC entrypoint, dependency wiring
internal/
  domain/           Entities + port interfaces (User, UserRepository, TokenService, PasswordHasher). No external deps.
  application/      Use cases orchestrating domain via ports
  config/           Environment variable loading
  adapters/
    http/           echo handlers, routes, middleware
    grpc/           gRPC server + generated userv1 code
    mongodb/        UserRepository implementation
    jwt/            TokenService implementation (HS256)
    bcrypt/         PasswordHasher implementation
    reporter/       Periodic user-count logger (the concurrency task)
```

Rule: `domain` and `application` never import adapter packages (echo, mongo-driver, jwt lib,
grpc). Adapters depend inward on `domain`/`application`, never the reverse.

## Prerequisites

- Go 1.25+
- Docker (required for `-tags=integration` tests, and to run the full stack via compose)
- [golangci-lint](https://golangci-lint.run/) v2
- `protoc` + `protoc-gen-go` + `protoc-gen-go-grpc` (only needed if you change `api/proto/user/v1/user.proto` and want to regenerate `internal/adapters/grpc/userv1`)

Install golangci-lint:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```

## Build

```bash
go build ./...
```

## Test

```bash
go test ./... -race -cover
```

Adapter packages that need a live dependency (MongoDB) are covered by a separate
integration suite, build-tagged `integration` so it's excluded from the default run above
and doesn't require Docker just to `go build`/unit-test:

```bash
go test -tags=integration ./... -v
```

Requires a running Docker daemon - these tests spin up real containers via
[testcontainers-go](https://golang.testcontainers.org/) (e.g. `mongo:7` for the MongoDB
adapter) rather than mocking the driver, so they exercise real behavior like unique-index
enforcement.

## Lint

```bash
golangci-lint run ./...
```

Config: `.golangci.yml`. Enforces `gofmt`/`goimports`, `errcheck`, `govet`, `staticcheck`,
`revive` (exported-symbol doc comments, ctx-first argument order), `gosec`, and more.

## Run

### With Docker Compose (recommended)

```bash
docker compose up --build
```

Starts MongoDB and the API together. The API listens on `:8080` (HTTP) and `:9090` (gRPC).
Mongo publishes no host port - it is reachable only from the API over the compose network,
since `mongo:7` runs without authentication. Override the JWT secret with an env var if you want something other than the dev default:

```bash
JWT_SECRET=some-real-secret docker compose up --build
```

### Locally

Requires a MongoDB instance reachable at `MONGO_URI` (e.g. `docker run -p 27017:27017 mongo:7`):

```bash
export MONGO_URI="mongodb://localhost:27017"
export JWT_SECRET="dev-secret-change-me"
go run ./cmd/api
```

## Environment Variables

| Variable | Required | Default | Notes |
| --- | --- | --- | --- |
| `MONGO_URI` | yes | - | MongoDB connection string |
| `MONGO_DATABASE` | no | `user_management` | Database name |
| `JWT_SECRET` | yes | - | HMAC secret used to sign/verify tokens |
| `JWT_TTL` | no | `24h` | Token lifetime, Go duration syntax (`1h`, `30m`, ...) |
| `HTTP_PORT` | no | `8080` | HTTP listen port |
| `GRPC_PORT` | no | `9090` | gRPC listen port |
| `USER_COUNT_LOG_INTERVAL` | no | `10s` | How often the background reporter logs the user count |

## API Reference

Base URL: `http://localhost:8080`. All request/response bodies are JSON.

| Method | Path | Auth | Description |
| --- | --- | --- | --- |
| GET | `/healthz` | none | Liveness check |
| POST | `/api/v1/auth/register` | none | Create a user |
| POST | `/api/v1/auth/login` | none | Authenticate, returns a JWT |
| GET | `/api/v1/users` | Bearer | List all users |
| GET | `/api/v1/users/:id` | Bearer | Fetch a user by id |
| PATCH | `/api/v1/users/:id` | Bearer | Update `name` and/or `email` |
| DELETE | `/api/v1/users/:id` | Bearer | Delete a user |

### Register

```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"name":"Jane Doe","email":"jane@example.com","password":"s3cret123"}'
```

```json
{
  "id": "01a05dc1-2caa-7b21-acb1-b5b6cdec6baf",
  "name": "Jane Doe",
  "email": "jane@example.com",
  "created_at": "2026-09-01T16:15:36.207329719Z"
}
```

`400` on invalid input (missing name, malformed email, password under 8 characters or
over 72 bytes), `409` if the email is already registered.

### Login

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"jane@example.com","password":"s3cret123"}'
```

```json
{ "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...." }
```

`401` on wrong email or password.

### List users

```bash
curl http://localhost:8080/api/v1/users \
  -H "Authorization: Bearer $TOKEN"
```

```json
[
  { "id": "01a0...", "name": "Jane Doe", "email": "jane@example.com", "created_at": "2026-09-01T16:15:36.207Z" }
]
```

### Get a user

```bash
curl http://localhost:8080/api/v1/users/01a05dc1-2caa-7b21-acb1-b5b6cdec6baf \
  -H "Authorization: Bearer $TOKEN"
```

`404` if the id doesn't exist.

### Update a user

```bash
curl -X PATCH http://localhost:8080/api/v1/users/01a05dc1-2caa-7b21-acb1-b5b6cdec6baf \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Jane Updated"}'
```

Returns the updated user. `400` if neither `name` nor `email` is present, `404` if the
user doesn't exist, `409` if the new email is already taken.

### Delete a user

```bash
curl -X DELETE http://localhost:8080/api/v1/users/01a05dc1-2caa-7b21-acb1-b5b6cdec6baf \
  -H "Authorization: Bearer $TOKEN"
```

`204` on success, `404` if the user doesn't exist.

## JWT Guide

1. **Register** a user via `POST /api/v1/auth/register`.
2. **Login** via `POST /api/v1/auth/login` with the same email/password. The response body
   is `{"token": "..."}`.
3. **Use the token** on every protected endpoint by sending it as a Bearer token:

   ```
   Authorization: Bearer <token>
   ```

4. Tokens are signed with HS256 using `JWT_SECRET` and expire after `JWT_TTL` (default 24h).
   The claims are the standard registered claims: `sub` (user id), `iat`, `exp`.
5. A missing, malformed, expired, or wrong-secret token gets a `401`:

   ```json
   { "error": "invalid or expired token" }
   ```

   A missing/malformed `Authorization` header gets a `401` with `"missing or malformed authorization header"`.

## gRPC

Proto: `api/proto/user/v1/user.proto`, service `user.v1.UserService` with `CreateUser` and
`GetUser` RPCs, mirroring the register/get-user HTTP endpoints and backed by the same
application use cases. Listens on `GRPC_PORT` (default `9090`). No auth metadata is
required on the gRPC path - the spec calls token metadata optional, and the HTTP path
already demonstrates JWT-protected access.

### Calling it with grpcurl

Server reflection is not registered, so point `grpcurl` at the `.proto` file. Install it
with `go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest`, then, from this
directory with the stack running:

```bash
# CreateUser - same rules and errors as POST /users
grpcurl -plaintext -import-path . -proto api/proto/user/v1/user.proto \
  -d '{"name":"Jane Doe","email":"jane@example.com","password":"s3cret123"}' \
  localhost:9090 user.v1.UserService/CreateUser
```

```json
{
  "id": "68b8f0c1e4b0a1d2c3e4f5a6",
  "name": "Jane Doe",
  "email": "jane@example.com",
  "createdAt": "2026-09-04T09:15:22Z"
}
```

```bash
# GetUser - by the id returned above
grpcurl -plaintext -import-path . -proto api/proto/user/v1/user.proto \
  -d '{"id":"68b8f0c1e4b0a1d2c3e4f5a6"}' \
  localhost:9090 user.v1.UserService/GetUser
```

Errors map to gRPC status codes: `InvalidArgument` for validation failures (including a
password over 72 bytes), `AlreadyExists` for a duplicate email, `NotFound` for an unknown
id, `Internal` for anything unexpected.

```
ERROR:
  Code: AlreadyExists
  Message: email already exists
```

`grpcurl describe user.v1.UserService` (with the same `-import-path`/`-proto` flags) prints
the service definition without a running server.

Generated code lives in `internal/adapters/grpc/userv1` and is checked in; regenerate it
after editing the `.proto` with:

```bash
protoc -I . -I "$(brew --prefix protobuf)/include" \
  --go_out=. --go_opt=module=github.com/neverholiday/backend-challenge/solutions/user-management-api \
  --go-grpc_out=. --go-grpc_opt=module=github.com/neverholiday/backend-challenge/solutions/user-management-api \
  api/proto/user/v1/user.proto
```

## Continuous Integration

`.github/workflows/ci.yml` (at the repository root) runs on every branch push:

| Job | What it does |
| --- | --- |
| `build and unit tests` | `go mod tidy` diff check, `go build`, `go vet` with and without the `integration` tag, `go test -race`, coverage profile uploaded as an artifact |
| `golangci-lint` | `golangci-lint` v2.13.2 against `.golangci.yml`, config schema validated first |
| `integration tests` | `go test -tags=integration -race`, using the runner's Docker daemon for the testcontainers `mongo:7` instance |
| `docker image builds` | Builds the `Dockerfile` (no push) with a GitHub Actions layer cache |

The Go version comes from `go.mod` via `go-version-file`, so there is one place to bump it.

## Design Decisions / Assumptions

- **Hexagonal architecture** chosen over a simpler layered approach: satisfies the bonus
  requirement and keeps business logic (`domain`/`application`) independent of MongoDB,
  echo, grpc, and the JWT library - testable without any of them.
- **echo** as HTTP framework.
- **Domain repository port is `context.Context`-first** on every method, so cancellation/
  timeouts propagate from HTTP request down to MongoDB calls, and graceful shutdown can
  cut off in-flight work cleanly.
- **Sentinel domain errors** (`ErrUserNotFound`, `ErrEmailAlreadyExists`, `ErrInvalidCredentials`,
  `ErrInvalidToken`) instead of string matching, so adapters/HTTP/gRPC layers can map errors
  to status codes via `errors.Is`.
- **Password length is capped at 72 bytes** (`domain.MaxPasswordLength`). bcrypt hashes at
  most 72 bytes and returns an error beyond that, so an overlong password would otherwise
  surface as a `500`. The limit is stated in `domain` because it is a rule callers must
  respect to use the `PasswordHasher` port; the bcrypt adapter maps the library's
  `ErrPasswordTooLong` onto `domain.ErrPasswordTooLong`, and both the HTTP (`400`) and gRPC
  (`InvalidArgument`) adapters map that sentinel to a client error. Silently truncating to
  72 bytes was rejected: it would make two different passwords authenticate the same
  account.
- **bcrypt cost**: `bcrypt.DefaultCost` (10). No tuning knob was added since the challenge
  doesn't call for one and a fixed, well-known default is easy to reason about.
- **Login is timing-equalized.** Returning early when no user matched would answer an
  unknown email in microseconds while a known email with a wrong password costs a full
  bcrypt comparison (~60ms here) - a reliable oracle for which addresses are registered.
  `AuthenticateUser` therefore calls `PasswordHasher.CompareDummy` on the not-found branch,
  verifying against a placeholder hash and discarding the result, so both failures cost the
  same and return the same `401 invalid credentials`. Measured after the change: 62.79ms for
  a known email, 62.83ms for an unknown one.
- **Registration still reveals whether an email is taken** (`409`), which is a weaker form
  of the same enumeration. It is kept because the alternative - accepting the registration
  and reporting success either way - needs an email-confirmation flow that is out of scope
  here. Noted rather than hidden.
- **Authentication, but no per-user authorization.** Any valid token may read, update, or
  delete any user - the challenge defines no roles, ownership, or admin concept, and adding
  one would be inventing requirements. The token's subject is available to handlers
  (`authMiddleware` puts it on the context), so an ownership check is a single comparison
  away when the product defines who may act on whom. Stated explicitly because a reviewer
  should see it as a decision rather than an oversight.
- **JWT claims**: only the standard registered claims (`sub`, `iat`, `exp`) are used. `sub`
  is the user's ID; no roles/scopes exist in the domain model, so there was nothing else to
  put in the token.
- **PATCH semantics**: `UpdateUser` returns the full updated user rather than a bare
  200/204, so a client doesn't need a follow-up `GET`. The repository port returns the
  updated user from the write itself (`FindOneAndUpdate` with `ReturnDocument(After)`)
  instead of the handler reading the row back: one round trip, and a concurrent update
  cannot land in between and make the response show state this request never wrote.
- **Validation lives in `domain`, applied by the use cases** (`domain.ValidateName`,
  `ValidateEmail`, `ValidatePassword`, `UserUpdateParam.Validate`), not in each adapter.
  What counts as a valid user is a business rule, and duplicating it per adapter lets them
  drift: an account created over gRPC under looser rules would be unusable over HTTP.
  Adapters only map the resulting `*domain.ValidationError` to their transport - HTTP `400`,
  gRPC `InvalidArgument`. The HTTP adapter keeps its own small validator for what only it
  can see (a malformed JSON body) and for login, which has no gRPC counterpart.
- **Validation is hand-rolled** (`net/mail` for email shape, length checks) rather than
  pulling in a validation library - the field set is small enough that a dependency would
  add more surface area than it saves.
- **Logging** uses `log/slog` with a custom middleware (method, path, status, duration)
  rather than echo's built-in logger, so the fields match the challenge's requirement
  exactly and output is structured JSON.
- **Concurrency task** is a standalone, independently-testable `UserCountReporter` type
  (constructed with the repo, interval, and logger) rather than an inline ticker loop in
  `main.go`, so its start/stop behavior can be unit tested without a real server.
- **gRPC** shares the same `RegisterUser`/`GetUser` use cases as HTTP - no business logic is
  duplicated between the two adapters.
- Bonuses implemented: Docker/docker-compose, input validation, graceful shutdown, gRPC
  (`CreateUser`/`GetUser`).
