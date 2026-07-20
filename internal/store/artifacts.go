package store

import (
	"database/sql"
	"errors"
)

// SaveArtifact stores a generated output file (e.g. the xlsx workbook) for a
// result. A result completes once, so each (result, kind) is written once.
func (s *Store) SaveArtifact(resultID int64, kind string, content []byte) error {
	_, err := s.db.Exec(
		`INSERT INTO result_artifacts(result_id, kind, content, created_at) VALUES(?, ?, ?, ?)`,
		resultID, kind, content, nowTS())
	return err
}

// ArtifactKinds returns the kinds of stored artifacts for a result, in insert
// order.
func (s *Store) ArtifactKinds(resultID int64) ([]string, error) {
	rows, err := s.db.Query(`SELECT kind FROM result_artifacts WHERE result_id = ? ORDER BY id`, resultID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var kinds []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		kinds = append(kinds, k)
	}
	return kinds, rows.Err()
}

// Artifact returns the stored content for a result artifact.
func (s *Store) Artifact(resultID int64, kind string) ([]byte, error) {
	var content []byte
	err := s.db.QueryRow(
		`SELECT content FROM result_artifacts WHERE result_id = ? AND kind = ?`, resultID, kind).Scan(&content)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return content, nil
}
