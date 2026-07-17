package store

import (
	"database/sql"
	"errors"
)

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

func (s *Store) UserByUsername(username string) (User, error) {
	row := s.db.QueryRow(
		`SELECT id, username, name, organisation, pw_hash, created_at
		   FROM users WHERE username = ? COLLATE NOCASE`, username)
	return scanUser(row)
}

func (s *Store) UserByID(id int64) (User, error) {
	row := s.db.QueryRow(
		`SELECT id, username, name, organisation, pw_hash, created_at
		   FROM users WHERE id = ?`, id)
	return scanUser(row)
}

// UpdateProfile sets the display name and organisation for a user.
func (s *Store) UpdateProfile(id int64, name, organisation string) error {
	_, err := s.db.Exec(
		`UPDATE users SET name = ?, organisation = ? WHERE id = ?`,
		name, organisation, id)
	return err
}

func scanUser(row *sql.Row) (User, error) {
	var u User
	var ts string
	err := row.Scan(&u.ID, &u.Username, &u.Name, &u.Organisation, &u.PwHash, &ts)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	u.CreatedAt = parseTS(ts)
	return u, nil
}
