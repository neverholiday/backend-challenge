# User Management API

RESTful user management API in Go, backed by MongoDB, with JWT (HS256) authentication.
Built with hexagonal architecture (ports & adapters).

## Status

Work in progress. This README is updated as parts land.

| Layer | Status |
| --- | --- |
| Domain (`internal/domain`) | Done — `User` entity, `UserRepository` port, sentinel errors |
| Application (use cases) | Pending |
| Adapters (HTTP, MongoDB, JWT, bcrypt) | Pending |
| gRPC | Pending |
| Docker / docker-compose | Pending |

## Architecture

Hexagonal (ports & adapters):

```
cmd/
  api/            HTTP entrypoint, dependency wiring
  grpc/           gRPC entrypoint (planned)
internal/
  domain/         Entities + port interfaces (User, UserRepository). No external deps.
  application/     Use cases orchestrating domain via ports (planned)
  adapters/
    http/          echo handlers, routes, middleware (planned)
    grpc/           gRPC server (planned)
    mongodb/        UserRepository implementation (planned)
    jwt/             TokenService implementation (planned)
    bcrypt/          PasswordHasher implementation (planned)
```

Rule: `domain` and `application` never import adapter packages (echo, mongo-driver, jwt lib).
Adapters depend inward on `domain`/`application`, never the reverse.

## Prerequisites

- Go 1.25+
- Docker + Docker Compose (for running MongoDB / full stack, once added)
- [golangci-lint](https://golangci-lint.run/) v2

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

## Lint

```bash
golangci-lint run ./...
```

Config: `.golangci.yml`. Enforces `gofmt`/`goimports`, `errcheck`, `govet`, `staticcheck`,
`revive` (exported-symbol doc comments, ctx-first argument order), `gosec`, and more.

## Run

```
TODO — once cmd/api is wired to real dependencies (Mongo connection, JWT secret from env).
```

## Environment Variables

```
TODO — documented once config loading is implemented.
```

## API Reference

```
TODO — endpoint list, sample requests/responses, once handlers are implemented.
```

## JWT Guide

```
TODO — how to obtain and use a token, once auth endpoints are implemented.
```

## Design Decisions / Assumptions

- **Hexagonal architecture** chosen over a simpler layered approach: satisfies the bonus
  requirement and keeps business logic (`domain`/`application`) independent of MongoDB,
  echo, and the JWT library — testable without any of them.
- **echo** as HTTP framework.
- **Domain repository port is `context.Context`-first** on every method, so cancellation/
  timeouts propagate from HTTP request down to MongoDB calls, and graceful shutdown can
  cut off in-flight work cleanly.
- **Sentinel domain errors** (`ErrUserNotFound`, `ErrEmailAlreadyExists`) instead of string
  matching, so adapters/HTTP layer can map errors to status codes via `errors.Is`.
- Bonuses in scope: Docker/docker-compose, input validation, graceful shutdown, gRPC
  (`CreateUser`/`GetUser`).
