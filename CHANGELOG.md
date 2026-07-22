# Changelog

All notable changes to AssayManager are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

The authoritative version lives in `main.go` (the `Version` constant) and must
match the latest entry below. Every code change gets a patch bump and a new
entry here.

## [0.2.3] - 2026-07-21

### Added
- Bulk assay **export/import** on the Assays page (no client-side JS):
  - Checkbox per assay + "Export selected (JSON/YAML)" buttons download one file
    (envelope `{"format":1,"assays":[…]}`) containing the **latest version** of
    each selected assay.
  - An import form accepts such a JSON or YAML file and inserts its assays,
    **preserving each assay's version** and **skipping any (name, version) already
    present** (idempotent — safe to re-import). Oligos are re-derived from
    `seqActual` and the header is validated on import. A summary flash reports
    added / skipped / failed counts. Intended for backup/restore across a DB
    reset. `store.ImportAssay` added.

## [0.2.2] - 2026-07-21

### Added
- BLAST reference source for analysis runs. On the Run page, the reference source
  can be **NCBI BLAST** (in addition to a FASTA upload). BLAST pulls the target
  taxIDs (`tgtTaxids`) and query region (`refAmpliconSeq`) from the selected
  assay; the publication **date range** is entered per-run. The run invokes the
  tool with `--ref-source blast --blast-query … --blast-taxid … --ncbi-email …`.
  Requires an NCBI contact email (`AM_NCBI_EMAIL`); the BLAST option is hidden and
  runs are rejected when it isn't set. No API key is used yet.
- Per-user **BLAST tuning** on the profile page: min coverage, min identity, and
  hitlist size (`users` columns, defaults 0.9 / 0.6 / 20000), applied to that
  user's BLAST runs.

### Changed
- `analysis.Request` gained a `Blast` variant and `analysis.NewCLI` takes the
  NCBI email/tool; argument construction moved to a unit-tested `buildArgs`
  (verified without any network/tool call). `store.UpdateProfile` now takes a
  `Profile` struct. `users` table gained BLAST-tuning columns (delete the DB file
  to apply).

## [0.2.1] - 2026-07-21

### Added
- `.env` support: a gitignored `.env` file in the working directory is read at
  startup (custom stdlib parser — `KEY=VALUE`, `#` comments, quotes, CRLF-safe,
  optional `export` prefix). Real OS environment variables take precedence; a
  missing file is not an error. Committed `example.env` as a template.
- Config for the (upcoming) BLAST reference source: `AM_NCBI_EMAIL`,
  `AM_NCBI_TOOL` (default `AssayManager`), `AM_NCBI_API_KEY`. The email is logged
  at startup when set; the API key is never logged.

## [0.2.0] - 2026-07-20

Milestone release (human-requested minor bump): the file-based inclusivity
analysis MVP is complete and tested end-to-end (see 0.1.6–0.1.7). No functional
change beyond the version bump.

### Added
- `README.md` describing the repository structure and responsibilities.

## [0.1.7] - 2026-07-20

### Added
- Result display reworked to match the tool's Excel output style:
  - **Mismatch patterns table** with one column per oligo (forward/probe/reverse)
    showing each oligo's per-pattern signature, plus count, percentage,
    cumulative %, total mismatches, matched counts, amplicon length, and example
    sequences. (Ports the tool's signature-splitting.)
  - **Per-class mismatch distribution** (forward/probe/reverse × 0/1/>1/no-match)
    shown as `count (pct%)`, and the overall breakdown as percentages.
- Downloadable outputs: each completed run now generates and stores the tool's
  Excel (.xlsx), text (.txt), and JSON files (`result_artifacts` table, blobs,
  cascade-deleted with the result); served from `GET /results/{id}/download/{kind}`
  with links on the result page. Generated at run time (JSON also served from the
  stored report). FASTA dumps deferred.

### Changed
- The analysis run now invokes the tool with `--json --xlsx --txt` to a temp
  outdir and reads the files back (instead of `--emit-json-stdout`); `Report`
  carries the captured artifacts. Parsed `Result` extended with `meta.oligos`
  and `summary.mismatch_distribution`, plus a `Table()` display view-model.

## [0.1.6] - 2026-07-20

### Added
- File-based inclusivity analysis (real, replacing the stub). `internal/analysis`
  now runs the `inclusivity_check_blast` binary as a subprocess: it writes the
  assay (AssayManager JSON, parsed by the tool directly) to a temp file and runs
  it with `--emit-json-stdout --no-config -q` against an uploaded reference
  FASTA, capturing the consolidated JSON from stdout.
  - Startup health check via `--capabilities`; the run feature is disabled
    (not fatal) if the binary is missing or its `schema_version` != 1. Binary
    path resolves the configured location, falling back to `.exe` on Windows.
  - Run form now takes a **reference FASTA upload** (multipart, streamed to a
    temp file, cap `AM_MAX_REF_UPLOAD`, default 50 MiB) plus the assay version
    and optional notes.
  - Pre-run analysis-eligibility gate: ≥1 forward + ≥1 reverse primer (with a
    non-empty clean sequence) and unique oligo names.
  - Background runs are bounded by a semaphore (`AM_MAX_CONCURRENT_RUNS`,
    default 2) and time-limited (`AM_ANALYSIS_TIMEOUT`, default 30m); the run row
    is created immediately and filled in on completion, per the MVP model.
  - Results store the raw consolidated JSON plus provenance (reference name,
    tool name/version, schema version); the result view renders a structured
    summary + top patterns, falling back to raw JSON.
- Config: `MaxReferenceUploadBytes`, `AnalysisTimeout`, `MaxConcurrentRuns`
  (with `AM_*` env overrides).

### Changed
- `analysis.Analyzer` reworked around `Request{AssayJSON, ReferencePath}` /
  `Report{RawJSON, tool meta}` with an `Available()` method; the text `Stub` was
  removed. `results` table gained `reference_name`, `tool_name`, `tool_version`,
  `schema_version` columns (delete the DB file to apply).

## [0.1.5] - 2026-07-17

### Added
- Assay header now has an optional `description` field (free-text comment):
  added to `AssayHeader` (`internal/assayparser`), the `MkHeader` constructor,
  the new-assay YAML skeleton, and the assay detail view. It round-trips through
  JSON/YAML automatically and is not required by validation. Documented in
  `assay_format.md`.

## [0.1.4] - 2026-07-17

### Added
- Assay editor: a structured "Add oligo" section (name, function dropdown, actual
  sequence) that appends a correctly-formatted oligo to the YAML on submit
  (`POST /assays/add-oligo`), reloading the page. No client-side JS — the current
  textarea content is submitted with the request, so in-progress edits are kept;
  the new oligo is built via the assayparser so its clean sequence and mods are
  derived. The add-oligo fields are preserved across preview/add and cleared on
  successful add.

## [0.1.3] - 2026-07-17

### Added
- File logging: events are now written to an append-only log file (default
  `assaymanager.log` in the working directory; configurable via `-log` /
  `AM_LOG`) in addition to the console. Explicit "server session started" and
  "server session stopped" events bracket each run (stop is logged on graceful
  Ctrl+C / SIGTERM shutdown). The file is appended across restarts so session
  history is retained.

## [0.1.2] - 2026-07-17

### Added
- Initial web application MVP (server-rendered, no client-side JS), organised as:
  - `internal/config` — flags/env configuration.
  - `internal/store` — SQLite data layer (`modernc.org/sqlite`, WAL) behind a
    repository API; users, assays, and results tables; portable SQL (a move to
    Postgres is anticipated). No migrations yet — delete the DB file to reset.
  - `internal/auth` — bcrypt password hashing, in-memory sessions, CSRF tokens.
  - `internal/analysis` — `Analyzer` interface with a `Stub` implementation; the
    real inclusivity_check_blast CLI integration comes later.
  - `internal/web` — routing, middleware (session, auth guard, CSRF, body cap,
    panic recovery), handlers, embedded HTML templates and stylesheet.
- User signup/login/logout and a profile page (name, organisation).
- Assay management: create/edit via a YAML editor with a server-rendered preview
  that derives clean sequences and modification lists via the assayparser.
  Immutable versioning (`vMAJOR.MINOR`, new lineage at `v0.1`; the user chooses a
  minor or major bump when saving under an existing name). Name+version are
  derived from the JSON header (authoritative). List, view, history, and delete.
- Analysis runs using the goroutine model: a results row is created immediately
  (status `running`); a background goroutine runs the stub and writes the outcome
  on completion. A `Scheduled checks` placeholder page.
- Dependency: `golang.org/x/crypto` (bcrypt), `modernc.org/sqlite`.

### Notes
- CSRF protection covers authenticated POST forms; login/register are not yet
  CSRF-protected (no pre-auth token). Session cookies are not `Secure` (local
  HTTP); enable once served over HTTPS.

## [0.1.1] - 2026-07-17

### Changed
- Reworked `internal/assayparser` modification handling to match the intended
  design:
  - `MkOligo` now returns an error and rejects unknown modifications,
    unterminated `/.../` markers, and invalid characters (previously they were
    silently dropped or could panic).
  - Modification positions are now 1-based **clean-sequence** coordinates;
    base-acting mods occupy their base position, non-base mods are anchored to
    the count of preceding bases (0 = 5' end).
  - Non-base modifications (fluorophores, quenchers, spacers) are supported and
    contribute no base to the clean sequence.
  - `Modification.Content` now stores the modification token (was hard-coded
    empty).
  - Renamed the `IsBase` field to `ActsAsBase` on `Modification` and
    `ModTemplate` (holds the base a mod stands in for, or `-` for non-base).
  - Seeded `ModCatalogue` with common non-base qPCR mods (kept hard-coded).
- Added oligo function-role constants (`forward-primer`, `reverse-primer`,
  `probe`) and `IsValidFunction`.

### Added
- Real unit tests for the assay parser (modification parsing, error cases,
  JSON/YAML round-trip); the previous `main_test.go` was empty.

## [0.1.0] - 2026-07-16

### Added
- Initial repository scaffolding: `main.go` with the authoritative `Version`
  constant, `CLAUDE.md` with agent rules, and this changelog.
- Integrated the `internal/assayparser` package (assay definition parsing,
  validation, and JSON/YAML I/O) into the `AssayManager` module. Removed its
  nested `go.mod`/`go.sum` so it builds and imports as a normal component
  (`AssayManager/internal/assayparser`).
- Added dependency `gopkg.in/yaml.v3 v3.0.1` (used by the assayparser I/O).
