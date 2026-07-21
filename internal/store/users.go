package store

import (
	"database/sql"
	"errors"
)

// Profile holds the user-editable profile fields, including per-user BLAST
// tuning applied to that user's BLAST runs.
type Profile struct {
	Name             string
	Organisation     string
	BlastMinCoverage float64
	BlastMinIdentity float64
	BlastHitlistSize int
}

// CreateUser inserts a new user with an empty profile and returns its id.
// The username's UNIQUE constraint enforces (case-insensitive) uniqueness.
func (s *Store) CreateUser(username, pwHash string) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO users(username, pw_hash, created_at) VALUES(?, ?, ?)`,
		username, pwHash, nowTS(),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UsernameTaken reports whether a username already exists.
func (s *Store) UsernameTaken(username string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM users WHERE username = ? COLLATE NOCASE`, username).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

const userCols = `SELECT id, username, name, organisation, pw_hash, created_at,
	blast_min_coverage, blast_min_identity, blast_hitlist_size FROM users`

func (s *Store) UserByUsername(username string) (User, error) {
	row := s.db.QueryRow(userCols+` WHERE username = ? COLLATE NOCASE`, username)
	return scanUser(row)
}

func (s *Store) UserByID(id int64) (User, error) {
	row := s.db.QueryRow(userCols+` WHERE id = ?`, id)
	return scanUser(row)
}

// UpdateProfile sets the editable profile fields for a user.
func (s *Store) UpdateProfile(id int64, p Profile) error {
	_, err := s.db.Exec(
		`UPDATE users
		    SET name = ?, organisation = ?,
		        blast_min_coverage = ?, blast_min_identity = ?, blast_hitlist_size = ?
		  WHERE id = ?`,
		p.Name, p.Organisation, p.BlastMinCoverage, p.BlastMinIdentity, p.BlastHitlistSize, id)
	return err
}

func scanUser(row *sql.Row) (User, error) {
	var u User
	var ts string
	err := row.Scan(&u.ID, &u.Username, &u.Name, &u.Organisation, &u.PwHash, &ts,
		&u.BlastMinCoverage, &u.BlastMinIdentity, &u.BlastHitlistSize)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	u.CreatedAt = parseTS(ts)
	return u, nil
}
