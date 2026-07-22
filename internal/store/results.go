package store

import (
	"database/sql"
	"sort"
)

// NewRun describes an analysis run being started (the reference source and any
// per-run notes/date range).
type NewRun struct {
	Params        string
	ReferenceName string
	Source        string // "file" | "blast"
	BlastFrom     string // YYYY/MM/DD, blast only, optional
	BlastTo       string
}

// CreateRun records the start of an analysis run against a specific assay
// version and returns the new result id. The run begins in the "running" state;
// the caller fills in the outcome later via CompleteRun or FailRun.
func (s *Store) CreateRun(ownerID int64, assay Assay, r NewRun) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO results(owner_id, assay_id, assay_name, assay_version, reference_name, source, blast_from, blast_to, status, params, started_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ownerID, assay.ID, assay.Name, assay.Version, r.ReferenceName, r.Source, r.BlastFrom, r.BlastTo, StatusRunning, r.Params, nowTS())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CompleteRun marks a run finished successfully and stores its report and the
// tool provenance (name, version, JSON schema version).
func (s *Store) CompleteRun(id int64, report, toolName, toolVersion string, schemaVersion int) error {
	_, err := s.db.Exec(
		`UPDATE results
		    SET status = ?, report = ?, tool_name = ?, tool_version = ?, schema_version = ?, finished_at = ?
		  WHERE id = ?`,
		StatusDone, report, toolName, toolVersion, schemaVersion, nowTS(), id)
	return err
}

// FailRun marks a run failed and stores the error message.
func (s *Store) FailRun(id int64, errMsg string) error {
	_, err := s.db.Exec(
		`UPDATE results SET status = ?, error = ?, finished_at = ? WHERE id = ?`,
		StatusFailed, errMsg, nowTS(), id)
	return err
}

// ListResults returns a user's runs, newest first.
func (s *Store) ListResults(ownerID int64) ([]Result, error) {
	rows, err := s.db.Query(resultCols+` WHERE owner_id = ?`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Result
	for rows.Next() {
		r, err := scanResult(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out, nil
}

// ResultByID returns a single run, scoped to the owner.
func (s *Store) ResultByID(ownerID, id int64) (Result, error) {
	rows, err := s.db.Query(resultCols+` WHERE id = ? AND owner_id = ?`, id, ownerID)
	if err != nil {
		return Result{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Result{}, err
		}
		return Result{}, ErrNotFound
	}
	return scanResult(rows)
}

// RecentDoneResults returns the most recent completed runs for the owner,
// newest first, capped at limit.
func (s *Store) RecentDoneResults(ownerID int64, limit int) ([]Result, error) {
	rows, err := s.db.Query(
		resultCols+` WHERE owner_id = ? AND status = ? ORDER BY started_at DESC LIMIT ?`,
		ownerID, StatusDone, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Result
	for rows.Next() {
		r, err := scanResult(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

const resultCols = `SELECT id, owner_id, assay_id, assay_name, assay_version, reference_name,
	source, blast_from, blast_to, status, params, report, error, tool_name, tool_version,
	schema_version, started_at, finished_at
	FROM results`

func scanResult(rows *sql.Rows) (Result, error) {
	var r Result
	var started string
	var finished sql.NullString
	err := rows.Scan(&r.ID, &r.OwnerID, &r.AssayID, &r.AssayName, &r.AssayVersion, &r.ReferenceName,
		&r.Source, &r.BlastFrom, &r.BlastTo, &r.Status, &r.Params, &r.Report, &r.Error,
		&r.ToolName, &r.ToolVersion, &r.SchemaVersion, &started, &finished)
	if err != nil {
		return Result{}, err
	}
	r.StartedAt = parseTS(started)
	if finished.Valid && finished.String != "" {
		t := parseTS(finished.String)
		r.FinishedAt = &t
	}
	return r, nil
}
