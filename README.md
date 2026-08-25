# AssayManager

A Go web server for storing, versioning, and analysing qPCR assays (oligos,
targets, modifications). It renders its own frontend for now; the HTTP layer is
kept thin so it can later back a JSON API.

## Structure

| Path | Responsibility |
|------|----------------|
| `main.go` | Entry point: wires config → store → server, sets up logging, holds the authoritative `Version`. |
| `internal/config` | Configuration from flags, `AM_*` env vars, and an optional `.env` file. |
| `internal/store` | SQLite data layer behind a repository API — users, assays, results, result artifacts, and scheduled jobs. Portable SQL (a Postgres move is anticipated). |
| `internal/auth` | bcrypt password hashing, in-memory sessions, CSRF tokens. |
| `internal/assayparser` | Parses/validates assay definitions, converts JSON⇄YAML, and derives clean sequences + modification lists from oligo sequences. |
| `internal/analysis` | Runs the external `inclusivity_check_blast` tool as a subprocess and parses/serves its output. |
| `internal/backup` | Ops-level periodic SQLite snapshots into a backup directory (configured via `backup.ini`; off by default). Snapshots to a local temp file and copies from there, so it works on network mounts. |
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
  recent runs. BLAST checks can also be **scheduled** to recur — a background
  scheduler runs due jobs on the latest version of the assay, resolving the
  look-back window against each run's date. It's optional — the run feature is
  disabled if the binary is absent, and BLAST additionally requires
  `AM_NCBI_EMAIL`. Concurrent runs are capped by `AM_MAX_CONCURRENT_RUNS`
  (default 1); since a run holds its slot for its whole duration, this also bounds
  how many BLAST/NCBI queries run at once.
- **Results.** Completed and in-progress checks are listed on the *Check results*
  page, filterable by assay and by the date the check was performed. Selected runs
  can be deleted — permanently, via a confirmation step that also removes their
  stored Excel/text/JSON downloads — or exported to a **PDF report**: a
  print-optimised page (opened in a new tab and saved from the browser's print
  dialog, A4 portrait) with a cover that lists the runs and their mismatch summary,
  a page break, then each run's full detailed view in sequence.
- **Backups** are an optional ops feature, off by default and not exposed in the
  UI. On startup the server reads (and, if absent, generates) a `backup.ini`
  beside the database; when enabled it takes periodic snapshots into a backup
  directory, tolerating an unavailable destination such as an unmounted network
  volume.
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
