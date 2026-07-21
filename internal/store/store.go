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
  pw_hash            TEXT NOT NULL,
  created_at         TEXT NOT NULL,
  blast_min_coverage REAL NOT NULL DEFAULT 0.9,
  blast_min_identity REAL NOT NULL DEFAULT 0.6,
  blast_hitlist_size INTEGER NOT NULL DEFAULT 20000
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

CREATE TABLE IF NOT EXISTS results (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  owner_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  assay_id       INTEGER NOT NULL REFERENCES assays(id) ON DELETE CASCADE,
  assay_name     TEXT NOT NULL,
  assay_version  TEXT NOT NULL,
  reference_name TEXT NOT NULL DEFAULT '',
  status         TEXT NOT NULL,
  params         TEXT NOT NULL DEFAULT '',
  report         TEXT NOT NULL DEFAULT '',
  error          TEXT NOT NULL DEFAULT '',
  tool_name      TEXT NOT NULL DEFAULT '',
  tool_version   TEXT NOT NULL DEFAULT '',
  schema_version INTEGER NOT NULL DEFAULT 0,
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
	_, err := s.db.Exec(schema)
	return err
}

// Timestamps are stored as RFC3339 UTC text for portability and to avoid
// driver-specific time handling.
func nowTS() string { return time.Now().UTC().Format(time.RFC3339) }

func parseTS(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
