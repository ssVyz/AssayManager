// Package store is the data-access layer. It wraps an SQLite database behind
// plain methods that return domain structs, so the HTTP layer never writes SQL
// directly. SQL is kept portable (no SQLite-only features in queries) because
// the database may later move to Postgres/Supabase.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a row does not exist (or is not owned by the
// requesting user).
var ErrNotFound = errors.New("not found")

// Run status values.
const (
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
)

type User struct {
	ID           int64
	Username     string
	Name         string
	Organisation string
	PwHash       string
	CreatedAt    time.Time

	// Per-user BLAST tuning (applied to this user's BLAST runs).
	BlastMinCoverage float64
	BlastMinIdentity float64
	BlastHitlistSize int

	// Number of recent runs shown on the dashboard.
	DashboardRunCount int

	// Dashboard mismatch-colour thresholds (percentages, 0–100). Each mismatch
	// cell on the dashboard is coloured green/yellow/red by which zone its
	// percentage lands in. Only the 0-mm bucket is a "higher is better" measure
	// (green is the high end); the 1-mm and >1-mm buckets are defect buckets
	// (worst-category mismatch counts), so a higher percentage is worse there
	// (green is the low end). "No match" is not coloured.
	Mm0Green float64 // 0 mm: at/above → green
	Mm0Warn  float64 // 0 mm: at/above → yellow (below → red)
	Mm1Green float64 // 1 mm: at/below → green
	Mm1Warn  float64 // 1 mm: at/below → yellow (above → red)
	Mm2Green float64 // >1 mm: at/below → green
	Mm2Warn  float64 // >1 mm: at/below → yellow (above → red)
}

type Assay struct {
	ID        int64
	OwnerID   int64
	Name      string
	Version   string
	Content   string // assay JSON — the authoritative form
	CreatedAt time.Time
}

type Result struct {
	ID            int64
	OwnerID       int64
	AssayID       int64
	AssayName     string
	AssayVersion  string
	ReferenceName string
	Source        string // "file" | "blast"
	BlastFrom     string // BLAST publication-date range (YYYY/MM/DD), if any
	BlastTo       string
	Status        string
	Params        string
	Report        string
	Error         string
	ToolName      string
	ToolVersion   string
	SchemaVersion int
	StartedAt     time.Time
	FinishedAt    *time.Time
}

// Schedule is a recurring analysis job. AssayName is joined for display and is
// not a stored column.
type Schedule struct {
	ID             int64
	OwnerID        int64
	AssayID        int64
	AssayName      string
	Method         string
	LookbackMonths int
	IntervalDays   int
	NextExecution  time.Time
	CreatedAt      time.Time
}

type Store struct{ db *sql.DB }

// Open opens (creating if needed) the SQLite database at path and ensures the
// schema exists. There is no migration system yet: when the schema changes,
// delete the database file and let it be recreated.
func Open(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// Serialize access: SQLite is single-writer, and one connection keeps the
	// MVP free of "database is locked" races without extra bookkeeping.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open db: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

const schema = `
CREATE TABLE IF NOT EXISTS users (
  id                 INTEGER PRIMARY KEY AUTOINCREMENT,
  username           TEXT NOT NULL UNIQUE COLLATE NOCASE,
  name               TEXT NOT NULL DEFAULT '',
  organisation       TEXT NOT NULL DEFAULT '',
  pw_hash             TEXT NOT NULL,
  created_at          TEXT NOT NULL,
  blast_min_coverage  REAL NOT NULL DEFAULT 0.9,
  blast_min_identity  REAL NOT NULL DEFAULT 0.6,
  blast_hitlist_size  INTEGER NOT NULL DEFAULT 20000,
  dashboard_run_count INTEGER NOT NULL DEFAULT 5,
  mm0_green           REAL NOT NULL DEFAULT 90,
  mm0_warn            REAL NOT NULL DEFAULT 70,
  mm1_green           REAL NOT NULL DEFAULT 10,
  mm1_warn            REAL NOT NULL DEFAULT 30,
  mm2_green           REAL NOT NULL DEFAULT 5,
  mm2_warn            REAL NOT NULL DEFAULT 20
);

CREATE TABLE IF NOT EXISTS assays (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  owner_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  version     TEXT NOT NULL,
  content     TEXT NOT NULL,
  created_at  TEXT NOT NULL,
  UNIQUE(owner_id, name, version)
);
CREATE INDEX IF NOT EXISTS idx_assays_owner_name ON assays(owner_id, name);

CREATE TABLE IF NOT EXISTS scheduled_jobs (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  owner_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  assay_id        INTEGER NOT NULL REFERENCES assays(id) ON DELETE CASCADE,
  method          TEXT NOT NULL DEFAULT 'blast',
  lookback_months INTEGER NOT NULL,
  interval_days   INTEGER NOT NULL,
  next_execution  TEXT NOT NULL,
  created_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sched_next ON scheduled_jobs(next_execution);

CREATE TABLE IF NOT EXISTS results (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  owner_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  assay_id       INTEGER NOT NULL REFERENCES assays(id) ON DELETE CASCADE,
  assay_name     TEXT NOT NULL,
  assay_version  TEXT NOT NULL,
  reference_name TEXT NOT NULL DEFAULT '',
  source         TEXT NOT NULL DEFAULT '',
  blast_from     TEXT NOT NULL DEFAULT '',
  blast_to       TEXT NOT NULL DEFAULT '',
  status         TEXT NOT NULL,
  params         TEXT NOT NULL DEFAULT '',
  report         TEXT NOT NULL DEFAULT '',
  error          TEXT NOT NULL DEFAULT '',
  tool_name      TEXT NOT NULL DEFAULT '',
  tool_version   TEXT NOT NULL DEFAULT '',
  schema_version INTEGER NOT NULL DEFAULT 0,
  schedule_id    INTEGER REFERENCES scheduled_jobs(id) ON DELETE SET NULL,
  started_at     TEXT NOT NULL,
  finished_at    TEXT
);
CREATE INDEX IF NOT EXISTS idx_results_owner ON results(owner_id);

CREATE TABLE IF NOT EXISTS result_artifacts (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  result_id  INTEGER NOT NULL REFERENCES results(id) ON DELETE CASCADE,
  kind       TEXT NOT NULL,
  content    BLOB NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(result_id, kind)
);
`

func (s *Store) migrate() error {
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	if err := s.ensureUserColumns(); err != nil {
		return err
	}
	// The 1-mismatch dashboard category switched from "higher is better" to
	// "higher is worse" (a defect bucket), which inverts the required threshold
	// ordering: green must now be ≤ warn. Reset any rows still in the old
	// arrangement (green > warn) to the new defaults so they aren't left with an
	// invalid, effectively all-green config. Idempotent: valid rows already
	// satisfy green ≤ warn and are untouched.
	if _, err := s.db.Exec(`UPDATE users SET mm1_green = 10, mm1_warn = 30 WHERE mm1_green > mm1_warn`); err != nil {
		return err
	}
	return nil
}

// ensureUserColumns adds user/profile columns introduced after the initial
// schema to pre-existing databases. Fresh databases already have them from
// `schema`; this makes the additive change non-destructive for older DB files
// (there is still no general migration system — see Open). Additive and
// idempotent: only missing columns are added.
func (s *Store) ensureUserColumns() error {
	have, err := s.tableColumns("users")
	if err != nil {
		return err
	}
	adds := []struct{ name, ddl string }{
		{"mm0_green", `ALTER TABLE users ADD COLUMN mm0_green REAL NOT NULL DEFAULT 90`},
		{"mm0_warn", `ALTER TABLE users ADD COLUMN mm0_warn REAL NOT NULL DEFAULT 70`},
		{"mm1_green", `ALTER TABLE users ADD COLUMN mm1_green REAL NOT NULL DEFAULT 10`},
		{"mm1_warn", `ALTER TABLE users ADD COLUMN mm1_warn REAL NOT NULL DEFAULT 30`},
		{"mm2_green", `ALTER TABLE users ADD COLUMN mm2_green REAL NOT NULL DEFAULT 5`},
		{"mm2_warn", `ALTER TABLE users ADD COLUMN mm2_warn REAL NOT NULL DEFAULT 20`},
	}
	for _, a := range adds {
		if have[a.name] {
			continue
		}
		if _, err := s.db.Exec(a.ddl); err != nil {
			return fmt.Errorf("add column %s: %w", a.name, err)
		}
	}
	return nil
}

// tableColumns returns the set of existing column names for a table. The table
// name is a trusted internal constant (never user input).
func (s *Store) tableColumns(table string) (map[string]bool, error) {
	rows, err := s.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols[name] = true
	}
	return cols, rows.Err()
}

// Timestamps are stored as RFC3339 UTC text for portability and to avoid
// driver-specific time handling.
func nowTS() string { return time.Now().UTC().Format(time.RFC3339) }

func parseTS(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
