# AssayManager

A Go web server for storing, versioning, and analysing qPCR assays (oligos,
targets, modifications). It renders its own frontend for now; the HTTP layer is
kept thin so it can later back a JSON API.

## Structure

| Path | Responsibility |
|------|----------------|
| `main.go` | Entry point: wires config → store → server, sets up logging, holds the authoritative `Version`. |
| `internal/config` | Configuration from flags, `AM_*` env vars, and an optional `.env` file. |
| `internal/store` | SQLite data layer behind a repository API — users, assays, results, and result artifacts. Portable SQL (a Postgres move is anticipated). |
| `internal/auth` | bcrypt password hashing, in-memory sessions, CSRF tokens. |
| `internal/assayparser` | Parses/validates assay definitions, converts JSON⇄YAML, and derives clean sequences + modification lists from oligo sequences. |
| `internal/analysis` | Runs the external `inclusivity_check_blast` tool as a subprocess and parses/serves its output. |
| `internal/web` | Routing, middleware (session, auth, CSRF, upload limits), handlers, embedded HTML templates + CSS. |
| `assets/` | The compiled `inclusivity_check_blast` binary (not committed). |

## Notes

- **Assays** are versioned immutably (`vMAJOR.MINOR`; each save is a new row). The
  JSON header is authoritative for name and version; DB columns are derived. They
  are created/edited in a YAML editor and can be bulk exported/imported as JSON or
  YAML (import preserves versions and skips duplicates).
- **Analysis** runs the external tool in the background against one of two
  reference sources — a user-uploaded FASTA, or an NCBI BLAST search (which takes
  the target taxIDs and reference amplicon from the assay) — storing the
  consolidated JSON plus downloadable Excel/text/JSON reports. Checks can be run
  one at a time or, for BLAST, on several assays at once; the dashboard summarises
  recent runs. It's optional — the run feature is disabled if the binary is
  absent, and BLAST additionally requires `AM_NCBI_EMAIL`.
- **No migrations yet:** delete the DB file to reset the schema.
- **Configuration** comes from flags and `AM_*` environment variables. For
  convenience, a gitignored `.env` file in the working directory is also read at
  startup (real env vars take precedence). Copy `example.env` to `.env` and fill
  in values — notably `AM_NCBI_EMAIL` to enable the BLAST reference source.
  Because `.env` is resolved relative to the working directory (and is not carried
  by `git`), when running under a service manager either set its working directory
  to the app directory or pass the `AM_*` vars directly.

## Build & run

- `go run .` — start the server (default `:8080`). Flags: `-addr`, `-db`, `-log`,
  `-inclusivity-bin` (env: `AM_ADDR`, `AM_DB`, `AM_LOG`, `AM_INCLUSIVITY_BIN`).
- `go build ./...` — build the module. `go test ./...` — run tests.
