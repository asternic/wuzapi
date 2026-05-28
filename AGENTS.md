# Agent Notes

## What This Repo Is

WuzAPI is a Go HTTP API wrapper around the `go.mau.fi/whatsmeow` library.
It exposes WhatsApp sessions, pairing, messaging, groups, contacts, media,
webhooks, and related admin flows as a REST API.

The API talks to WhatsApp through whatsmeow, which connects to WhatsApp's
WebSocket servers. The API itself does not use headless Chrome and does not
run WhatsApp in an emulator.

This project can interact with real WhatsApp accounts. Avoid spammy test
traffic, avoid repeated pairing attempts, and treat test numbers as disposable.

## Project Shape

This is a single Go module with a mostly flat `package main` at the repo root.
Important entry points:

- `main.go`: process startup, flags, config, server bootstrap.
- `routes.go`: HTTP route registration.
- `handlers.go`: most endpoint handlers. It is large; find the route first.
- `handlers_grouprequests.go`: group request handlers.
- `wmiau.go`: main whatsmeow integration layer.
- `db.go` and `migrations.go`: persistence and schema migration logic.
- `stdio.go` and `stdio_test.go`: stdio support and local unit tests.
- `API.md` and `wuzapi_postman.json`: API documentation/client artifacts.
- `e2e/`: physical Android WhatsApp end-to-end test suite.

## Language Conventions

All repository artifacts should be written in en-US, including code, comments,
tests, Gherkin scenarios, documentation updates, examples, and developer-facing
text.

## Commands

Use Go 1.25.x, matching `go.mod`.

```bash
go build .
go test .
go vet .
```

Do not use `go test ./...` as a casual validation command. It can include the
physical-device e2e package under `e2e/`.

To run locally:

```bash
cp .env.sample .env
go run .
```

Useful runtime flags are documented in `README.md`; common ones include
`-port`, `-address`, `-admintoken`, `-logtype`, `-color`, `-osname`,
`-skipmedia`, and `-wadebug`.

The app can use SQLite by default. PostgreSQL settings are available through
`DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_HOST`, `DB_PORT`, and `DB_SSLMODE`.
Docker Compose files exist for local and swarm setups.

## E2E Tests With Physical Android WhatsApp

The e2e suite runs the real WuzAPI server and drives a real physical Android
phone running WhatsApp through Appium/UiAutomator2 and adb. This is not a mock,
not an emulator-only test, and not safe to run accidentally.

Only run e2e tests when the user explicitly asks for them.

Requirements are documented in `e2e/README.md`. In short:

- Android Platform Tools available in `PATH` (`adb`).
- One authorized physical Android device visible in `adb devices`.
- WhatsApp installed and already configured/logged in on that device.
- Appium running at `E2E_APPIUM_URL` or `http://127.0.0.1:4723`.
- `ANDROID_HOME` or `ANDROID_SDK_ROOT` exported before starting local Appium.
- `E2E_PAIR_PHONE` set to the primary WhatsApp number, digits only, no `+`.

The e2e command is:

```bash
go test ./e2e
```

WhatsApp UI labels change. If Appium cannot find controls, inspect selectors
under `e2e/whatsapp/**/selectors.go` before assuming the API flow is broken.

## Change Guidance

- Keep API behavior, `API.md`, and `wuzapi_postman.json` aligned when endpoints
  change.
- Keep route additions in `routes.go` and endpoint logic near the existing
  handler patterns.
- Put schema changes in `migrations.go`; do not rely on ad-hoc SQL notes.
- Do not commit `.env`, `.e2e-runtime/`, WhatsApp session data, tokens, or
  device-specific configuration.
- Prefer small, targeted edits. This repo has large files, so use search first
  and avoid broad formatting churn.
- Before touching WhatsApp pairing, device names, linked devices, or selectors,
  read the matching e2e steps under `e2e/features/` and `e2e/whatsapp/`.
