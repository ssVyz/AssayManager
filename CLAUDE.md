# CLAUDE.md

Guidance for AI agents working in this repository.

## Project

AssayManager is a Go web server that acts as the application layer for managing
molecular assays (oligos, targets, modifications). For now it also serves the
frontend to users.

## Layout

- `main.go` — entry point; holds the authoritative `Version` constant.
- `internal/assayparser/` — a normal package of this module
  (`AssayManager/internal/assayparser`) that parses/validates assay definitions
  and handles JSON/YAML I/O. It was copied in from another repo; it has no
  separate versioning and is maintained as part of AssayManager.

## Rules for agents

1. **Versioning.** The authoritative version is `Version` in `main.go` (semver,
   starts at `0.1.0`). Bump the **patch** number for any code change you make.
   **Minor** and **major** bumps happen only when a human asks.
2. **Changelog.** Document every code change in `CHANGELOG.md` under a new
   version entry that matches the bumped `Version`.
3. **Git.** Never commit or push — humans handle all git operations.
4. **Dependencies.** Do not add new dependencies without asking first; explain
   why each is needed.

## Build

- `go run .` — runs the server (currently prints the version).
- `go build ./...` — builds the module.
