# Changelog

All notable changes to AssayManager are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

The authoritative version lives in `main.go` (the `Version` constant) and must
match the latest entry below. Every code change gets a patch bump and a new
entry here.

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
