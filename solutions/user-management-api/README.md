# User Management API

[![CI](https://github.com/neverholiday/backend-challenge/actions/workflows/ci.yml/badge.svg)](https://github.com/neverholiday/backend-challenge/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/neverholiday/backend-challenge/graph/badge.svg)](https://codecov.io/gh/neverholiday/backend-challenge)

RESTful user management API in Go. MongoDB for storage, JWT (HS256) for auth. Built with
hexagonal architecture (ports and adapters). It also serves a gRPC service for `CreateUser`
and `GetUser`.

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

Hexagonal (ports and adapters):

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

The rule: `domain` and `application` never import adapter packages (echo, mongo-driver, jwt
lib, grpc). Adapters point inward to `domain` and `application`, never the other way.

## Prerequisites

- Go 1.25+
- Docker (needed for `-tags=integration` tests, and to run the full stack via compose)
- [golangci-lint](https://golangci-lint.run/) v2
- `protoc` + `protoc-gen-go` + `protoc-gen-go-grpc` (only if you change `api/proto/user/v1/user.proto` and want to regenerate `internal/adapters/grpc/userv1`)

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

The number in the badge comes from CI, not from me typing it here. Both test jobs upload a
profile to [Codecov](https://codecov.io/gh/neverholiday/backend-challenge): the unit run
under the `unit` flag, the integration run under `integration`. Counting both is the only
honest total, because the MongoDB adapter is covered by the integration job alone.
`codecov.yml` leaves out the generated `userv1` package and `cmd/api`, which is wiring.

Adapters that need a live dependency (MongoDB) sit in a separate suite. I build-tag it
`integration` so the default run above stays fast and you don't need Docker just to build
or unit test:

```bash
go test -tags=integration ./... -v
```

You need a running Docker daemon for this one. The tests start real containers with
[testcontainers-go](https://golang.testcontainers.org/), for example `mongo:7` for the
MongoDB adapter, so they hit real behavior like unique index enforcement.

I mock at the ports, not at the driver.

Use cases, HTTP handlers, gRPC handlers and the reporter run against in-memory fakes of
`UserRepository`, `TokenService` and `PasswordHasher`. That is what the ports are for. No
Docker needed, and I can make a fake fail on demand to reach error paths a real dependency
won't give me when I ask.

The MongoDB adapter I test for real with `mongo:7`. All it does is translate to and from
the driver. A mocked driver would only prove the call happened, not that it was right: it
would still pass with an unmapped duplicate key error, a missing `FindOneAndUpdate` option,
or a misspelled BSON tag.

## Lint

```bash
golangci-lint run ./...
```

Config: `.golangci.yml`. It enforces `gofmt`/`goimports`, `errcheck`, `govet`,
`staticcheck`, `revive` (doc comments on exported symbols, ctx-first argument order),
`gosec`, and more.

## Run

### With Docker Compose (recommended)

```bash
docker compose up --build
```

This starts MongoDB and the API together. The API listens on `:8080` for HTTP and `:9090`
for gRPC. Mongo publishes no host port, so only the API reaches it over the compose
network. That is on purpose, because `mongo:7` here runs without authentication. Set your
own JWT secret if you don't want the dev default:

```bash
JWT_SECRET=some-real-secret docker compose up --build
```

### Locally

You need MongoDB reachable at `MONGO_URI`, for example
`docker run -p 27017:27017 mongo:7`:

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

Base URL: `http://localhost:8080`. All request and response bodies are JSON.

Every error looks the same, at every status, so a client needs one branch to read a
failure:

```json
{ "error": "user not found" }
```

`GET /healthz` returns `{"status":"ok"}` and needs no auth. The compose healthcheck polls
it.

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

`400` on bad input: missing name, malformed email, password under 8 characters or over 72
bytes. `409` if the email is already registered.

### Login

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"jane@example.com","password":"s3cret123"}'
```

```json
{ "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...." }
```

`401` on a wrong email or a wrong password.

### List users

```bash
curl http://localhost:8080/api/v1/users \
  -H "Authorization: Bearer $TOKEN"
```

```json
[
  {
    "id": "01a05dc1-2caa-7b21-acb1-b5b6cdec6baf",
    "name": "Jane Doe",
    "email": "jane@example.com",
    "created_at": "2026-09-01T16:15:36.207329719Z"
  }
]
```

With no users you get `[]`, not `null`.

### Get a user

```bash
curl http://localhost:8080/api/v1/users/01a05dc1-2caa-7b21-acb1-b5b6cdec6baf \
  -H "Authorization: Bearer $TOKEN"
```

```json
{
  "id": "01a05dc1-2caa-7b21-acb1-b5b6cdec6baf",
  "name": "Jane Doe",
  "email": "jane@example.com",
  "created_at": "2026-09-01T16:15:36.207329719Z"
}
```

`404` if the id doesn't exist:

```json
{ "error": "user not found" }
```

### Update a user

```bash
curl -X PATCH http://localhost:8080/api/v1/users/01a05dc1-2caa-7b21-acb1-b5b6cdec6baf \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Jane Updated"}'
```

```json
{
  "id": "01a05dc1-2caa-7b21-acb1-b5b6cdec6baf",
  "name": "Jane Updated",
  "email": "jane@example.com",
  "created_at": "2026-09-01T16:15:36.207329719Z"
}
```

You get the full updated user back, so no follow-up `GET`. `400` if you send neither `name`
nor `email` (`{"error":"at least one of name or email is required"}`). `404` if the user
doesn't exist. `409` if the new email is already taken.

### Delete a user

```bash
curl -X DELETE http://localhost:8080/api/v1/users/01a05dc1-2caa-7b21-acb1-b5b6cdec6baf \
  -H "Authorization: Bearer $TOKEN"
```

`204` with an empty body on success. `404` with `{"error":"user not found"}` if the user
doesn't exist.

## JWT Guide

1. **Register** a user with `POST /api/v1/auth/register`.
2. **Login** with `POST /api/v1/auth/login` using the same email and password. You get back
   `{"token": "..."}`.
3. **Send the token** on every protected endpoint as a Bearer token:

   ```
   Authorization: Bearer <token>
   ```

4. Tokens are signed with HS256 using `JWT_SECRET` and expire after `JWT_TTL`, 24h by
   default. The claims are the standard registered ones: `sub` (user id), `iat`, `exp`.
5. A missing, malformed, expired or wrong-secret token gets a `401`:

   ```json
   { "error": "invalid or expired token" }
   ```

   A missing or malformed `Authorization` header gets a `401` with
   `"missing or malformed authorization header"`.

## gRPC

Proto: `api/proto/user/v1/user.proto`, service `user.v1.UserService` with `CreateUser` and
`GetUser`. They mirror the register and get-user HTTP endpoints and run the same use cases.
It listens on `GRPC_PORT`, `9090` by default. I don't require auth metadata here: the spec
calls token metadata optional, and the HTTP side already shows JWT protection.

### Calling it with grpcurl

I don't register server reflection, so point `grpcurl` at the `.proto` file. Install it
with `go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest`, then run from this
directory with the stack up:

```bash
# CreateUser - same rules and errors as POST /api/v1/auth/register
grpcurl -plaintext -import-path . -proto api/proto/user/v1/user.proto \
  -d '{"name":"Jane Doe","email":"jane@example.com","password":"s3cret123"}' \
  localhost:9090 user.v1.UserService/CreateUser
```

```json
{
  "id": "01a05dc1-2caa-7b21-acb1-b5b6cdec6baf",
  "name": "Jane Doe",
  "email": "jane@example.com",
  "createdAt": "2026-09-01T16:15:36.207329719Z"
}
```

```bash
# GetUser - by the id returned above
grpcurl -plaintext -import-path . -proto api/proto/user/v1/user.proto \
  -d '{"id":"01a05dc1-2caa-7b21-acb1-b5b6cdec6baf"}' \
  localhost:9090 user.v1.UserService/GetUser
```

Errors map to gRPC codes: `InvalidArgument` for validation failures, a password over 72
bytes included, `AlreadyExists` for a duplicate email, `NotFound` for an unknown id, and
`Internal` for anything I didn't expect.

```
ERROR:
  Code: AlreadyExists
  Message: email already exists
```

`grpcurl describe user.v1.UserService`, with the same `-import-path` and `-proto` flags,
prints the service definition without a running server.

The generated code sits in `internal/adapters/grpc/userv1` and is checked in. Regenerate it
after you edit the `.proto`:

```bash
protoc -I . -I "$(brew --prefix protobuf)/include" \
  --go_out=. --go_opt=module=github.com/neverholiday/backend-challenge/solutions/user-management-api \
  --go-grpc_out=. --go-grpc_opt=module=github.com/neverholiday/backend-challenge/solutions/user-management-api \
  api/proto/user/v1/user.proto
```

## Continuous Integration

`.github/workflows/ci.yml`, at the repository root, runs on every branch push:

| Job | What it does |
| --- | --- |
| `build and unit tests` | `go mod tidy` diff check, `go build`, `go vet` with and without the `integration` tag, `go test -race`, coverage profile uploaded as an artifact and to Codecov under the `unit` flag |
| `golangci-lint` | `golangci-lint` v2.13.2 against `.golangci.yml`, config schema validated first |
| `integration tests` | `go test -tags=integration -race`, using the runner's Docker daemon for the testcontainers `mongo:7` instance, coverage uploaded under the `integration` flag |
| `docker image builds` | Builds the `Dockerfile`, no push, with a GitHub Actions layer cache |

The Go version comes from `go.mod` via `go-version-file`, so there is one place to bump it.

## Design Decisions / Assumptions

- **"Create a user" is `POST /api/v1/auth/register`**, not a second protected
  `POST /api/v1/users`. The spec lists user creation twice, once as registration and once
  as a user operation, and one handler covers both. If create were protected, only an
  existing user could make the first account, and the challenge gives me no admin role to
  solve that. gRPC `CreateUser` is the same use case on the other transport.
- **Hexagonal architecture** instead of a plain layered one. It covers the bonus, and it
  keeps `domain` and `application` free of MongoDB, echo, grpc and the JWT library. I can
  test them without any of those.
- **echo** for HTTP.
- **The repository port takes `context.Context` first** on every method. Cancellation and
  timeouts travel from the HTTP request down to MongoDB, and graceful shutdown can cut off
  in-flight work.
- **Sentinel domain errors** (`ErrUserNotFound`, `ErrEmailAlreadyExists`,
  `ErrInvalidCredentials`, `ErrInvalidToken`) instead of matching strings, so the HTTP and
  gRPC layers map errors to status codes with `errors.Is`.
- **Passwords are capped at 72 bytes** (`domain.MaxPasswordLength`). bcrypt hashes at most
  72 bytes and errors past that, so a longer password would come back as a `500`. The cap
  lives in `domain` because it is a rule callers must respect to use the `PasswordHasher`
  port. The bcrypt adapter maps the library's `ErrPasswordTooLong` onto
  `domain.ErrPasswordTooLong`, and HTTP (`400`) and gRPC (`InvalidArgument`) turn that into
  a client error. I rejected silent truncation to 72 bytes: two different passwords would
  then open the same account.
- **bcrypt cost** is `bcrypt.DefaultCost` (10). The challenge doesn't ask for tuning and a
  fixed, well known default is easy to reason about.
- **Login costs the same either way.** Returning early when no user matches answers an
  unknown email in microseconds, while a known email with a wrong password pays a full
  bcrypt compare, about 60ms here. That is a reliable oracle for which addresses are
  registered. So `AuthenticateUser` calls `PasswordHasher.CompareDummy` on the not-found
  branch, verifies against a placeholder hash and throws the result away. Both failures
  cost the same and return the same `401 invalid credentials`. Measured after the change:
  62.79ms for a known email, 62.83ms for an unknown one.
- **Registration still reveals whether an email is taken** with `409`. That is a weaker
  version of the same enumeration. I kept it, because the alternative is accepting the
  registration and reporting success either way, and that needs an email confirmation flow
  which is out of scope here. I would rather write it down than hide it.
- **Authentication, but no per-user authorization.** Any valid token can read, update or
  delete any user. The challenge defines no roles, no ownership and no admin, and adding
  one would be inventing requirements. The token subject is on the context, put there by
  `authMiddleware`, so an ownership check is one comparison away once the product says who
  may act on whom. I state it here so a reviewer reads it as a decision, not as a miss.
- **JWT claims** are only the standard registered ones: `sub`, `iat`, `exp`. `sub` is the
  user id. There are no roles or scopes in the domain model, so there was nothing else to
  put in the token.
- **PATCH returns the full updated user** instead of a bare 200 or 204, so the client
  doesn't need a follow-up `GET`. The write itself returns it, `FindOneAndUpdate` with
  `ReturnDocument(After)`, rather than the handler reading the row back. That is one round
  trip, and no concurrent update can land in between and make the response show state this
  request never wrote.
- **Validation lives in `domain` and the use cases apply it** (`domain.ValidateName`,
  `ValidateEmail`, `ValidatePassword`, `UserUpdateParam.Validate`), not in each adapter.
  What counts as a valid user is a business rule. Duplicate it per adapter and they drift:
  an account created over gRPC under looser rules would be unusable over HTTP. The adapters
  only map `*domain.ValidationError` to their transport, HTTP `400` and gRPC
  `InvalidArgument`. The HTTP adapter keeps a small validator of its own for what only it
  can see, a malformed JSON body, and for login, which has no gRPC counterpart.
- **Validation is hand-rolled**, `net/mail` for the email shape plus length checks, instead
  of a validation library. The field set is small enough that a dependency would add more
  surface than it saves. One tradeoff to know about: `mail.ParseAddress` accepts the RFC
  5322 display name form, so `"Jane Doe <jane@example.com>"` passes. That is correct by the
  RFC and harmless here, since I store and compare the value as given and the unique index
  covers the whole string. A system that emails users would normalize to the bare address
  first.
- **Logging** uses `log/slog` with my own middleware (method, path, status, duration)
  instead of echo's built-in logger, so the fields match what the challenge asks for and
  the output is structured JSON.
- **The concurrency task** is a `UserCountReporter` type, built from the repo, an interval
  and a logger, instead of an inline ticker loop in `main.go`. I can unit test its start
  and stop behavior without a real server.
- **gRPC** runs the same `RegisterUser` and `GetUser` use cases as HTTP, so no business
  logic is duplicated between the two adapters.
- Bonuses done: Docker and docker-compose, input validation, graceful shutdown, gRPC
  (`CreateUser`/`GetUser`).
