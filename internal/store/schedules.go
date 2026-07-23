package store

import (
	"database/sql"
	"time"
)

// CreateSchedule inserts a recurring job and returns its id.
func (s *Store) CreateSchedule(ownerID, assayID int64, method string, lookbackMonths, intervalDays int, nextExecution time.Time) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO scheduled_jobs(owner_id, assay_id, method, lookback_months, interval_days, next_execution, created_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?)`,
		ownerID, assayID, method, lookbackMonths, intervalDays, nextExecution.UTC().Format(time.RFC3339), nowTS())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListSchedules returns a user's schedules (with the anchor assay's name),
// soonest next-execution first.
func (s *Store) ListSchedules(ownerID int64) ([]Schedule, error) {
	rows, err := s.db.Query(
		`SELECT j.id, j.owner_id, j.assay_id, a.name, j.method, j.lookback_months, j.interval_days, j.next_execution, j.created_at
		   FROM scheduled_jobs j JOIN assays a ON a.id = j.assay_id
		  WHERE j.owner_id = ?
		  ORDER BY j.next_execution ASC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSchedules(rows)
}

// DueSchedules returns all schedules (across users) whose next execution is at
// or before now. The scheduler runs globally.
func (s *Store) DueSchedules(now time.Time) ([]Schedule, error) {
	rows, err := s.db.Query(
		`SELECT j.id, j.owner_id, j.assay_id, a.name, j.method, j.lookback_months, j.interval_days, j.next_execution, j.created_at
		   FROM scheduled_jobs j JOIN assays a ON a.id = j.assay_id
		  WHERE j.next_execution <= ?
		  ORDER BY j.next_execution ASC`, now.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSchedules(rows)
}

// AdvanceSchedule sets a schedule's next execution time.
func (s *Store) AdvanceSchedule(id int64, next time.Time) error {
	_, err := s.db.Exec(`UPDATE scheduled_jobs SET next_execution = ? WHERE id = ?`,
		next.UTC().Format(time.RFC3339), id)
	return err
}

// DeleteSchedule removes a schedule, scoped to the owner. Past results keep
// their schedule_id set to NULL (ON DELETE SET NULL), preserving history.
func (s *Store) DeleteSchedule(ownerID, id int64) error {
	_, err := s.db.Exec(`DELETE FROM scheduled_jobs WHERE id = ? AND owner_id = ?`, id, ownerID)
	return err
}

func scanSchedules(rows *sql.Rows) ([]Schedule, error) {
	var out []Schedule
	for rows.Next() {
		var j Schedule
		var next, created string
		if err := rows.Scan(&j.ID, &j.OwnerID, &j.AssayID, &j.AssayName, &j.Method,
			&j.LookbackMonths, &j.IntervalDays, &next, &created); err != nil {
			return nil, err
		}
		j.NextExecution = parseTS(next)
		j.CreatedAt = parseTS(created)
		out = append(out, j)
	}
	return out, rows.Err()
}
