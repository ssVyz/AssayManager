# Changelog

All notable changes to AssayManager are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

The authoritative version lives in `main.go` (the `Version` constant) and must
match the latest entry below. Every code change gets a patch bump and a new
entry here.

## [0.3.14] - 2026-08-28

### Changed
- The pattern-detail selector on the single-result view is now a one-click
  simple/full toggle (two plain links, current mode highlighted) instead of a
  dropdown needing a separate "Show" submit — switching a result's pattern table
  takes one click and still works without JavaScript.
- The selector in the results list is labelled "PDF pattern", since it applies to
  the exported report only; per-result switching happens in the result view.

## [0.3.13] - 2026-08-28

### Added
- A **pattern-detail selector** for the mismatch-pattern table, on the web view
  and in the printable report: `simple` (the new default) or `full`. In simple
  mode the per-oligo signature columns are replaced by one verdict column per
  oligo class — "Forward primer(s)", "Probe(s)", "Reverse primer(s)" — reading
  `perfect`, `N mm` or `none`; the best oligo of a class wins, so a class counts
  as a perfect match as soon as one of its oligos matches without mismatches.
  Everything else about the table (pattern number, counts, percentages, total
  mismatches, amplicon, examples) is unchanged, so rows still line up 1:1 with
  the full view and the Excel workbook. This keeps the table readable for assays
  with many oligos, where the full signatures wrapped or ran off the page.
  `full` renders exactly what the table did before.
  - The single-result view has a "Pattern" dropdown next to the table
    (`?pattern=simple|full`); the results list has one next to "Export PDF" that
    is carried into the report URL, so the PDF matches what was chosen.
  - `analysis.ResultView` now carries both projections (`ClassCols` +
    `PatternRow.ClassCells` alongside `OligoCols` + `PatternRow.Cells`). The
    per-class mismatch counts are derived from the tool's signature strings
    (every non-`.` character is a mismatch, as the tool counts them), because the
    consolidated JSON has no per-class best-mismatch field.
  - The downloadable outputs (xlsx, txt, JSON) are the analysis tool's own and
    are untouched — they always contain the full pattern detail.

## [0.3.12] - 2026-08-26

### Added
- Containerisation: a minimal two-stage `Dockerfile` (plus `.dockerignore`). The
  build stage compiles the server into a static binary and writes a `.env` file
  from a required `NCBI_EMAIL` build arg; the runtime stage is a
  `distroless/static-debian12` image (no shell, ~2 MB) holding the server, the
  prebuilt static `inclusivity_check_blast` Linux binary (copied from `assets/`
  and renamed), and the `.env`. The SQLite database, log and backups are written
  to the working directory as usual, persisting across container stop/start (but
  not across removal). No new Go dependencies.

## [0.3.11] - 2026-08-25

### Changed
- The result "Summary" section (on the single-result view and in the PDF report)
  now renders as a plain label/value table instead of four card-like tiles, for
  consistency with the surrounding distribution and overall tables.

## [0.3.10] - 2026-08-23

### Added
- PDF export of check results. Select one or more runs on the Check results page
  and click "Export PDF" to open a print-optimised report in a new tab, then use
  the browser's "Save as PDF" (A4 portrait). The report opens with a cover that
  states the export date and lists the exported runs with a mismatch sneak-peek
  (the same summary shown on the dashboard), followed by a page break and then the
  full detailed view of each run in sequence, one per page. No new dependency —
  the report is server-rendered HTML with a print stylesheet.

### Changed
- The result detail view (metadata, summary tiles, mismatch distribution, overall
  and pattern tables) moved into a shared template partial so the on-screen result
  page and the PDF report render from one source. On the single-result page the
  Downloads links now sit just below the heading, above the run metadata.

## [0.3.9] - 2026-08-23

### Added
- The Check results page now supports selecting runs with per-row checkboxes and
  a "select all" header box (the same pattern used on the Assays page).
- "Delete selected" removes runs — the first user way to delete results. It is a
  two-step action: selecting runs and clicking delete opens a confirmation page
  that lists exactly what will be removed and warns that deletion is permanent
  (and also drops each run's stored downloads); a final confirm actually deletes.
  Deletion is owner-scoped, and stored artifacts are cleaned up via the existing
  `result_artifacts` foreign-key cascade.
- A filter on the Check results page: narrow the list by assay and/or by the date
  range the check was performed (inclusive of both endpoints). Date bounds are
  interpreted in the server's local time to match the times shown in the table.

## [0.3.8] - 2026-08-20

### Fixed
- Backups to a network mount (NFS/CIFS) failed with `database is locked
  (SQLITE_BUSY)` and never wrote a file. `VACUUM INTO` creates a real SQLite
  database and takes file locks on it, and those locks are unreliable on network
  filesystems. The runner now writes the snapshot to a local temp file (beside
  the live database, on a lock-capable filesystem) and then copies the finished,
  static file to the destination with plain byte I/O — no SQLite locking touches
  the share. The copy goes to a `.part` temp name and is renamed into place, so a
  partial copy never appears as a valid backup, and the local temp is removed
  afterwards. Failures now record which stage failed (snapshot / copy / publish).

## [0.3.7] - 2026-08-20

### Added
- Automatic database backups (ops-level, not exposed in the web UI). Backups use
  SQLite's `VACUUM INTO` to write a consistent, standalone snapshot — safe to run
  against the live WAL-mode database, unlike copying the file. Configuration is a
  small self-documenting INI file (`backup.ini`) generated beside the database on
  first startup with backups **disabled** by default; it is read only at startup.
  Settings: `enabled`, `dir` (destination), `interval` (Go duration, default
  `24h`). Whether a backup is due is derived from the timestamp in the newest
  backup's filename (`<db-name>-YYYYMMDDThhmmssZ.db`), and each attempt is
  appended to a backup log in the destination directory.
  - The destination directory is **never created automatically** and must already
    exist — typically a mounted network volume. If it is missing at startup or
    backup time (e.g. the volume is not mounted), the server logs a warning and
    keeps running, retrying on later checks and resuming automatically when the
    directory reappears. This deliberately avoids silently writing backups to
    local disk at an unmounted mountpoint.
  - No automatic retention: backup files accumulate and are pruned manually.
  - Restore: stop the server, replace the live database with a chosen backup,
    delete any stale `-wal`/`-shm` files, and restart.

## [0.3.6] - 2026-08-20

### Fixed
- A run could get stuck in the "running" state forever and wedge the whole run
  queue: with the default single run slot, one stalled analysis (e.g. a BLAST
  request that hangs on NCBI) held the slot indefinitely, blocking every
  subsequent manual and scheduled run, and a restart did not clear the stranded
  row. Two independent safeguards now prevent this:
  - **Startup reconciliation.** On boot the server fails every run still marked
    "running" (`store.FailStaleRuns`, logged with a count). Such a run can only
    be an orphan from a previous process, so the DB is made honest and the queue
    always starts clean — regardless of how the last process ended.
  - **Independent run watchdog.** Each analysis now runs in a child goroutine
    supervised by a watchdog that frees the queue slot and marks the run failed
    even if the analysis goroutine itself wedges and never returns. The analysis
    timeout still kills the subprocess; the watchdog is the backstop for a kill
    that fails to unblock the goroutine.

### Changed
- Default per-run analysis timeout lowered from 30 to 15 minutes
  (`AM_ANALYSIS_TIMEOUT`). A run is terminated 15 minutes after it starts; the
  watchdog abandons a wedged run one minute later.

## [0.3.5] - 2026-08-13

### Added
- Dashboard runs list can now be filtered by assay. A toggle above the list
  switches between "Recent" (the default — the most recent completed runs across
  all assays, exactly as before) and "Assay" (pick one assay from a dropdown to
  see only its completed checks). Assay mode shows the full history for that
  assay across all its versions, newest first (not capped by the profile's
  recent-runs setting). Implemented as a plain GET form, so it works without
  JavaScript.

## [0.3.4] - 2026-08-13

### Added
- "Select all" checkboxes for the multi-select lists: the Assays list (export)
  and the Run check → Batch BLAST list. A master checkbox in each table header
  toggles every checkbox in that table and reflects the group state
  (checked/indeterminate) as rows are ticked individually.
- This is the app's first client-side script (`static/app.js`, loaded with
  `defer`) — a genuine "check all" can't be done in HTML/CSS alone. It is
  progressive enhancement: without JavaScript, the individual checkboxes work
  exactly as before.

## [0.3.3] - 2026-08-13

### Fixed
- Dashboard colour thresholds: the 1-mismatch category is now treated as
  "higher is worse" (a defect bucket), matching >1 mm, instead of "higher is
  better". The three mismatch buckets are computed from each sequence's worst
  category and are mutually exclusive (0 / exactly 1 / ≥2), so a rising
  1-mismatch percentage is bad and must colour red. Its profile inputs now read
  "at or below", validation requires green ≤ warn, and the default cutoffs are
  10/30 (green/yellow). The dashboard column header is relabelled "≤1 mm" → "1
  mm" so the buckets read as a clean partition (0 mm / 1 mm / >1 mm).
- Startup migration resets any stored 1-mismatch thresholds left in the old
  arrangement (green > warn) to the new 10/30 defaults, so profiles saved under
  0.3.2 aren't left with an invalid, effectively all-green config.

## [0.3.2] - 2026-08-13

### Added
- Per-user dashboard colour thresholds. The dashboard mismatch cells (0 mm,
  ≤1 mm, >1 mm) are now coloured green/yellow/red by user-defined zones instead
  of fixed colours, so problem runs stand out at a glance. Each category has two
  cutoffs set on the profile page: for 0 mm and ≤1 mm a higher percentage is
  better (green is the high end); for >1 mm a higher percentage is worse (green
  is the low end). "No match" stays grey.
  - Stored as six new `users` columns (`mm0_green/mm0_warn`, `mm1_*`, `mm2_*`)
    with sensible defaults (0 mm: 90/70; ≤1 mm: 95/80; >1 mm: 5/20). Existing
    databases are upgraded in place by an additive, idempotent `ALTER TABLE`
    step at startup (no DB wipe needed).
  - Colour selection moved from fixed template classes into a unit-tested
    `mmClass` helper; the profile save validates the thresholds (0–100, green on
    the correct side of yellow per category).

## [0.3.1] - 2026-08-10

### Fixed
- BLAST runs now write the query sequence (`targets.refAmpliconSeq`) to a
  single-record FASTA file in the per-run temp dir and pass its path to
  `--blast-query`, instead of passing the sequence inline. The tool's
  file-or-inline heuristic only falls back to "inline" on a file-not-found
  error, so an inline query longer than the OS filename limit (~255 bytes) was
  misread as a too-long path and the run failed with "file name too long". Any
  assay with a reference amplicon ≥ 256 nt was affected; passing a file works
  for any query length. Applies to single, batch, and scheduled BLAST runs.

## [0.3.0] - 2026-07-24

### Added
- Modification reference in the structured editor: an inline, JS-free
  "Show available modifications" panel (`<details>`) next to the oligo table,
  listing every recognised modification code, the base it stands in for (if
  any), and a description. The list is generated from
  `assayparser.ModCatalogue`, so it always matches what the parser accepts.

## [0.2.9] - 2026-07-24

### Added
- Structured ("convenient") assay editor, now the default for New assay and Edit
  assay — a single field-based page (no YAML), with a Header section, an editable
  oligo table (per-row name/function/sequence with a live-derived clean
  sequence + mods, add-N-rows and per-row remove via server round-trips, no JS),
  and a Targets section exposing only the BLAST essentials (target taxIDs as a
  comma-separated field, reference amplicon). Fields the form doesn't expose
  (off-target taxIDs, amplicon source/size, search string) are preserved across
  edits via a hidden base. Inline help throughout.
- The YAML editor is retained as the "advanced" alternative; both editors switch
  to each other (`/assays/form/to-yaml`, `/assays/yaml/to-form`) preserving the
  in-progress assay. Both build the same `ValidAssay` and use the same
  save/version path.

## [0.2.8] - 2026-07-21

### Added
- Analysis scheduling (recurring BLAST checks). New `scheduled_jobs` table
  (owner, anchor assay, method, look-back months, interval days, next execution)
  and a `schedule_id` column on `results` (nullable, `ON DELETE SET NULL` so
  history survives schedule deletion). The Scheduled-checks page lists a user's
  schedules and creates new ones (BLAST-eligible assays only). A background
  scheduler goroutine (ticks each minute, started at boot, stops on shutdown)
  fires jobs whose next execution has passed: it advances the next execution to
  *now + interval* first (missed cycles are skipped, not replayed), then — if the
  job can run — starts a normal background run of the **latest version** of the
  anchor assay's lineage, with the look-back resolved against the run date.
  Jobs that can't run (assay gone/ineligible, BLAST unavailable) are silently
  skipped after advancing. Runs go through the same queue/cap as manual runs.
  (Schema change — delete the DB file to apply.)

## [0.2.7] - 2026-07-21

### Changed
- `AM_MAX_CONCURRENT_RUNS` now **defaults to 1** (was 2), so out of the box only
  one analysis run executes at a time — which, since a run holds its slot for its
  whole duration, keeps BLAST to a single concurrent NCBI query (polite to public
  NCBI). Documented in `example.env` and the README. Raise it (e.g. with your own
  BLAST DB) to allow more concurrent runs. The single cap governs file and BLAST
  runs alike — kept deliberately simple.

## [0.2.6] - 2026-07-21

### Changed
- BLAST publication-date selection now defaults to a **look-back period in months**
  (default 12, capped at 240), resolved to concrete `from`/`to` dates against the
  current date at submit time (bounded to today); a **custom date range** is the
  non-default alternative. Applies to both the single-run and batch-BLAST forms.
  Both resolved dates are sent to the tool and stored on the run, so downstream
  display is unchanged. This look-back is the primitive the upcoming analysis
  scheduling will persist and re-resolve per execution.

## [0.2.5] - 2026-07-21

### Added
- Dashboard "Recent runs" table: the most recent completed analysis runs (file
  and BLAST) with assay name, run time, date range, sequence count, and the
  overall mismatch breakdown — 0 / ≤1 / >1 mismatches plus no-match — as
  percentages with subtle green/yellow/red/grey colour coding. The number of
  runs shown is a per-user profile setting (`dashboard_run_count`, default 5,
  1–50).
- `results` rows now record the reference `source` ("file"/"blast") and the
  BLAST publication-date range, so the dashboard can show a clean date-range
  column. `store.CreateRun` now takes a `NewRun` struct; `store.RecentDoneResults`
  added. (Schema change — delete the DB file to apply.)

## [0.2.4] - 2026-07-21

### Added
- Batch BLAST runs. The Run check page now has a "Batch BLAST" section listing
  the latest version of each assay with its BLAST eligibility; select any number
  of eligible assays, set one shared publication-date range, and submit
  (`POST /run/batch`) to start one background BLAST run per assay. Ineligible
  assays are shown with a reason (no amplicon / no taxIDs / gate failure) and
  aren't selectable. Concurrency uses the existing run cap
  (`AM_MAX_CONCURRENT_RUNS`); the Check results page shows a started/skipped
  summary. File-source runs remain single (each needs its own upload).

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
