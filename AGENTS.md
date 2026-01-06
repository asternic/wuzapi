# General Guidelines

- Always open `specs.md` before working.
- Always update **Progress Tracker** after you finish your task.
- Always tick the corresponding milestone once it is complete.
- Await confirmation from the user if it can be considered done or not.

## Project Structure & Module Organization
- Go source files live at the repository root (e.g., `main.go`, `handlers.go`, `routes.go`).
- Static assets and web UI live under `static/` (dashboard, login, API docs, images).
- API reference and supporting docs are in `API.md` and `README.md`.
- Docker and service files are in `Dockerfile`, `docker-compose*.yml`, and `wuzapi.service`.

## Build, Test, and Development Commands
- `go build .` builds the `wuzapi` binary in the repo root.
- `go run .` runs the API server directly from source.
- `go test ./...` runs all Go tests (currently `stdio_test.go`).
- `docker compose up --build` builds and runs the service stack locally.

## Coding Style & Naming Conventions
- Format Go code with `gofmt` (tabs for indentation, standard Go formatting).
- Use Go naming conventions: exported identifiers in `PascalCase`, unexported in `camelCase`.
- Keep files and package names lowercase (this repo keeps most Go files at root).

## Testing Guidelines
- Use the Go testing package (`*_test.go` files). Example: `stdio_test.go`.
- Name tests as `TestXxx` and keep them close to the code they cover.
- Run `go test ./...` before opening a PR; add tests for new behavior.

## Commit & Pull Request Guidelines
- Commit messages in history are mixed; prefer concise, imperative messages.
- Conventional commits appear in places (e.g., `docs(README): ...`, `refactor: ...`) and are welcome.
- PRs should include a clear description, linked issues (if any), and notes on behavior or API changes.
- If you change endpoints or payloads, update `API.md` and `static/api/spec.yml`.

## Configuration & Security Notes
- Use `.env` for local configuration; start with `.env.sample`.
- Never commit secrets (tokens, keys, or credentials). `.env` is gitignored.
- For Docker Compose, set `DB_HOST=db` when using the bundled PostgreSQL service.
