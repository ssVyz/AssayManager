# CLAUDE.md

Guidance for AI agents working in this repository.

## Project

AssayManager is a Go web server that acts as the application layer for managing
molecular assays (oligos, targets, modifications). For now it also serves the
frontend to users.

## Layout

- `main.go` — entry point (wires config → store → server); holds the
  authoritative `Version` constant.
- `internal/config` — flags/env configuration.
- `internal/store` — SQLite data layer behind a repository API (portable SQL;
  Postgres move anticipated). No migrations yet: delete the DB file to reset.
- `internal/auth` — bcrypt hashing, in-memory sessions, CSRF tokens.
- `internal/analysis` — `Analyzer` interface + `Stub`; real CLI tool later.
- `internal/web` — routing, middleware, handlers, embedded templates + CSS.
- `internal/assayparser` — a normal package of this module that parses/validates
  assay definitions and handles JSON/YAML I/O. Copied in from another repo; no
  separate versioning, maintained as part of AssayManager.

## Architecture notes

- Handlers stay thin and call the store/analysis packages, so the same logic can
  later back a JSON API (the frontend may split off).
- The assay JSON header is authoritative for name+version; the DB columns are
  derived. Versions are immutable (`vMAJOR.MINOR`); each save is a new row.

## Rules for agents

1. **Versioning.** The authoritative version is `Version` in `main.go` (semver,
   starts at `0.1.0`). Bump the **patch** number for any code change you make.
   **Minor** and **major** bumps happen only when a human asks.
2. **Changelog.** Document every code change in `CHANGELOG.md` under a new
   version entry that matches the bumped `Version`.
3. **Git.** Never commit or push — humans handle all git operations.
4. **Dependencies.** Do not add new dependencies without asking first; explain
   why each is needed.

## Build & run

- `go run .` — starts the server (default `:8080`). Flags: `-addr`, `-db`,
  `-inclusivity-bin` (env: `AM_ADDR`, `AM_DB`, `AM_INCLUSIVITY_BIN`).
- `go build ./...` — builds the module. `go test ./...` — runs tests.
