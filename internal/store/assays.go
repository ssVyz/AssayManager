package store

import (
	"database/sql"
	"errors"
	"sort"

	"AssayManager/internal/assayparser"
)

// SaveNewVersion stores a new immutable version of an assay.
//
// The assay's identity is its name (per owner). If no version of that name
// exists yet, this creates the lineage at v0.1. Otherwise it bumps the latest
// version by bump ("minor" or "major"). The system-generated version is written
// into the JSON header, which is the authoritative source for name and version.
func (s *Store) SaveNewVersion(ownerID int64, a assayparser.ValidAssay, bump string) (Assay, error) {
	name := a.Header.Name

	latest, err := s.latestVersion(ownerID, name)
	if err != nil {
		return Assay{}, err
	}
	version := initialVersion
	if latest != "" {
		if version, err = bumpVersion(latest, bump); err != nil {
			return Assay{}, err
		}
	}

	a.Header.Version = version
	content, err := assayparser.ConvertJson(a)
	if err != nil {
		return Assay{}, err
	}

	ts := nowTS()
	res, err := s.db.Exec(
		`INSERT INTO assays(owner_id, name, version, content, created_at) VALUES(?, ?, ?, ?, ?)`,
		ownerID, name, version, string(content), ts)
	if err != nil {
		return Assay{}, err
	}
	id, _ := res.LastInsertId()
	return Assay{
		ID:        id,
		OwnerID:   ownerID,
		Name:      name,
		Version:   version,
		Content:   string(content),
		CreatedAt: parseTS(ts),
	}, nil
}

// latestVersion returns the highest version string for a lineage, or "" if the
// lineage does not exist yet.
func (s *Store) latestVersion(ownerID int64, name string) (string, error) {
	rows, err := s.db.Query(`SELECT version FROM assays WHERE owner_id = ? AND name = ?`, ownerID, name)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	best := ""
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return "", err
		}
		if best == "" || versionLess(best, v) {
			best = v
		}
	}
	return best, rows.Err()
}

// ListLineages returns the latest version of each distinct assay name owned by
// the user, sorted by name.
func (s *Store) ListLineages(ownerID int64) ([]Assay, error) {
	all, err := s.assaysForOwner(ownerID, "")
	if err != nil {
		return nil, err
	}
	latest := map[string]Assay{}
	for _, a := range all {
		if cur, ok := latest[a.Name]; !ok || versionLess(cur.Version, a.Version) {
			latest[a.Name] = a
		}
	}
	out := make([]Assay, 0, len(latest))
	for _, a := range latest {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ListVersions returns every version of a lineage, newest first.
func (s *Store) ListVersions(ownerID int64, name string) ([]Assay, error) {
	all, err := s.assaysForOwner(ownerID, name)
	if err != nil {
		return nil, err
	}
	sort.Slice(all, func(i, j int) bool { return versionLess(all[j].Version, all[i].Version) })
	return all, nil
}

// ListAllAssays returns every assay version owned by the user, sorted by name
// then newest version first. Useful for pickers (e.g. choosing a run target).
func (s *Store) ListAllAssays(ownerID int64) ([]Assay, error) {
	all, err := s.assaysForOwner(ownerID, "")
	if err != nil {
		return nil, err
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Name != all[j].Name {
			return all[i].Name < all[j].Name
		}
		return versionLess(all[j].Version, all[i].Version)
	})
	return all, nil
}

// AssayByID returns a specific assay version, scoped to the owner.
func (s *Store) AssayByID(ownerID, id int64) (Assay, error) {
	row := s.db.QueryRow(
		`SELECT id, owner_id, name, version, content, created_at
		   FROM assays WHERE id = ? AND owner_id = ?`, id, ownerID)
	var a Assay
	var ts string
	err := row.Scan(&a.ID, &a.OwnerID, &a.Name, &a.Version, &a.Content, &ts)
	if errors.Is(err, sql.ErrNoRows) {
		return Assay{}, ErrNotFound
	}
	if err != nil {
		return Assay{}, err
	}
	a.CreatedAt = parseTS(ts)
	return a, nil
}

// DeleteLineage removes all versions of a lineage (and, via cascade, their
// results). Returns the number of assay rows deleted.
func (s *Store) DeleteLineage(ownerID int64, name string) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM assays WHERE owner_id = ? AND name = ?`, ownerID, name)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// assaysForOwner returns assays for an owner, optionally filtered by name.
func (s *Store) assaysForOwner(ownerID int64, name string) ([]Assay, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if name == "" {
		rows, err = s.db.Query(
			`SELECT id, owner_id, name, version, content, created_at
			   FROM assays WHERE owner_id = ?`, ownerID)
	} else {
		rows, err = s.db.Query(
			`SELECT id, owner_id, name, version, content, created_at
			   FROM assays WHERE owner_id = ? AND name = ?`, ownerID, name)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Assay
	for rows.Next() {
		var a Assay
		var ts string
		if err := rows.Scan(&a.ID, &a.OwnerID, &a.Name, &a.Version, &a.Content, &ts); err != nil {
			return nil, err
		}
		a.CreatedAt = parseTS(ts)
		out = append(out, a)
	}
	return out, rows.Err()
}
