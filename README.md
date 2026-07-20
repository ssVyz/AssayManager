# AssayManager

A Go web server for storing, versioning, and analysing qPCR assays (oligos,
targets, modifications). It renders its own frontend for now; the HTTP layer is
kept thin so it can later back a JSON API.

## Structure

| Path | Responsibility |
|------|----------------|
| `main.go` | Entry point: wires config → store → server, sets up logging, holds the authoritative `Version`. |
| `internal/config` | Configuration from flags with env fallbacks. |
| `internal/store` | SQLite data layer behind a repository API — users, assays, results, and result artifacts. Portable SQL (a Postgres move is anticipated). |
| `internal/auth` | bcrypt password hashing, in-memory sessions, CSRF tokens. |
| `internal/assayparser` | Parses/validates assay definitions, converts JSON⇄YAML, and derives clean sequences + modification lists from oligo sequences. |
| `internal/analysis` | Runs the external `inclusivity_check_blast` tool as a subprocess and parses/serves its output. |
| `internal/web` | Routing, middleware (session, auth, CSRF, upload limits), handlers, embedded HTML templates + CSS. |
| `assets/` | The compiled `inclusivity_check_blast` binary (not committed). |

## Notes

- **Assays** are versioned immutably (`vMAJOR.MINOR`; each save is a new row). The
  JSON header is authoritative for name and version; DB columns are derived.
- **Analysis** runs the external tool against a user-uploaded reference FASTA in
  the background, storing the consolidated JSON plus downloadable Excel/text/JSON
  reports. It's optional — the run feature is disabled if the binary is absent.
- **No migrations yet:** delete the DB file to reset the schema.

## Build & run

- `go run .` — start the server (default `:8080`). Flags: `-addr`, `-db`, `-log`,
  `-inclusivity-bin` (env: `AM_ADDR`, `AM_DB`, `AM_LOG`, `AM_INCLUSIVITY_BIN`).
- `go build ./...` — build the module. `go test ./...` — run tests.
