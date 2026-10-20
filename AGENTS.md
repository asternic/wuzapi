# AGENTS.md

Shared guidance for coding agents working in this repository.

## Project overview

WuzAPI is a Go service that exposes WhatsApp functionality through an HTTP REST API and a JSON-RPC interface over stdin/stdout. It uses whatsmeow for the WhatsApp protocol, supports multiple users and device sessions, and distributes events through webhooks, RabbitMQ, and stdio notifications.

Use `go.mod` as the source of truth for Go and dependency versions. The current Go version is 1.26.0. WuzAPI relies on the pinned whatsmeow version; protocol changes can require coordinated dependency and application changes.

## Development commands

```bash
# Build without replacing the local workspace binary
go build -o /tmp/wuzapi-dev .

# HTTP mode (the default)
/tmp/wuzapi-dev -mode=http -address=0.0.0.0 -port=8080 -logtype=console -color=true

# JSON-RPC over stdin/stdout
/tmp/wuzapi-dev -mode=stdio

# Tests and static checks
go test ./...
go vet ./...
go test -race ./...

# Run selected tests
go test ./... -run 'TestStdioHealthRequest|TestResolveBlocklistPNJID' -count=1
```

There is an established test suite covering HTTP routing/authentication, stdio, sessions, history, media, proxy pools, and WhatsApp features. Reuse the existing helpers, including `makeTestServer`, and keep ordinary tests independent of live WhatsApp accounts. PostgreSQL concurrency tests require `WUZAPI_TEST_POSTGRES_DSN` pointing to a test database; they skip when it is unset.

CI runs dependency verification/tidying, `go vet ./...`, `go test -race ./...` with PostgreSQL, and a build. Keep `go.mod` and `go.sum` consistent.

Do not upgrade whatsmeow as routine setup. For an intentional upgrade, follow the maintainer section in `README.md`, adapt API changes, and run the relevant checks.

## Architecture

The application is a single `package main`, with feature-specific files alongside the original large handlers:

- `main.go`: flags, configuration, initialization, and HTTP/stdio startup.
- `routes.go`: HTTP routes and middleware wiring.
- `handlers.go` and `handlers_*.go`: API handlers and feature helpers, including business profiles, labels, polls, conversation settings, and message stars.
- `stdio.go`: explicit JSON-RPC method-to-HTTP mapping, authentication adaptation, responses, and event notifications.
- `clients.go`, `session_lifecycle.go`, `session_recovery.go`, `session_devices.go`: WhatsApp clients, connection lifecycle, recovery, and device ownership.
- `db.go`, `migrations.go`: application database setup, queries, and migrations.
- `history_sync.go`, `history_edit.go`, `message_delete.go`: history synchronization and message changes.
- `media*.go`, `s3manager.go`: attachment processing, temporary storage, delivery, and S3 integration. See `media-memory.md` before changing media buffering or resource ownership.
- `helpers.go`, `logging_config.go`, `proxy_pool.go`, `rabbitmq.go`: authentication/utilities, logging configuration, proxy allocation, and event publishing.
- `static/`: dashboard, API documentation, and browser assets.

Important dependencies include whatsmeow, Gorilla Mux, sqlx, zerolog, modernc SQLite, lib/pq, AWS SDK v2, and amqp091-go.

## Stdio contract

Stdio mode is the project's JSON-RPC bridge, not an MCP server. It invokes the same router and middleware as HTTP through in-process requests; it does not require an HTTP listener.

- Input is newline-delimited JSON, with one request per line and a 512 KiB scanner limit.
- Requests require an `id` and a supported `method`. IDs support strings and unsigned integer values. Requests without IDs are rejected.
- Methods are explicitly listed in `stdio.go`; they are not generated automatically from routes. For example, `chat.send.text` maps to `POST /chat/send/text`.
- `params.token` becomes the user `token` header; `params.adminToken` becomes the admin `Authorization` header. Preserve the existing payload filtering for strict endpoints and the use of `token` as data when creating users.
- Responses contain `jsonrpc: "2.0"`, the request ID, and either `result` or `error`. Existing errors commonly use HTTP status codes.
- Events can be emitted as outgoing notifications without IDs.
- Stdout must contain only protocol messages. Send logs to stderr and preserve the shared output lock so responses and notifications cannot interleave.

Example input:

```json
{"jsonrpc":"2.0","id":"1","method":"health"}
{"jsonrpc":"2.0","id":"2","method":"session.status","params":{"token":"USER_TOKEN"}}
```

Use `stdio.go` and `stdio_test.go` as the source of truth; the `JSON-RPC-API.md` mentioned in an older source comment is not present in this checkout.

## Configuration and persistence

Copy `.env.sample` for configuration examples and consult `README.md` for flags and environment variables. `WUZAPI_ADMIN_TOKEN` controls admin authentication; user endpoints use each user's token. Never include real tokens, credentials, passkeys, or pairing data in logs, examples, or tests.

SQLite is the default. PostgreSQL is selected when the required `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, and `DB_NAME` variables are populated; `DB_SSLMODE` controls SSL. Account for both database backends when changing queries or migrations.

Optional integrations include RabbitMQ, S3, webhook HMAC signing, and proxy pools. Preserve per-user isolation and session/device ownership when changing connection management.

WhatsApp identifiers can be phone-number JIDs or LIDs. Reuse the existing parsing and resolution helpers, preserve explicit server information, and do not assume every identifier can be converted to a phone number. LID handling does not imply support for @usernames.

## Common changes

When adding an API feature:

1. Register the HTTP route in `routes.go` and implement the handler, using a feature-specific file where appropriate.
2. Apply the existing authentication, validation, response conventions, and database abstractions.
3. Add the explicit method mapping in `stdio.go` when exposing the feature through stdio; verify payload and authentication handling.
4. Add meaningful regression tests using existing HTTP/stdio helpers and injectable operations where available.
5. Update the relevant API references (`API.md`, `static/api/spec.yml`, and `static/docs/index.html`), and dashboard or Postman examples when affected. Check existing documentation for the feature before editing.
6. Format changed Go files and run checks appropriate to the change.

For database changes, inspect `migrations.go` and existing backend-specific behavior. For media changes, preserve temporary-file cleanup, bounded memory use, and shutdown handling. For session changes, inspect the lifecycle and recovery regression tests.

## Deployment

Docker configuration lives in `Dockerfile` and `docker-compose.yml`; Swarm configuration is in `docker-compose-swarm.yaml`. A systemd unit is provided in `wuzapi.service`. See `README.md` for configuration and deployment details.
