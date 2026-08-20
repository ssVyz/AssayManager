package store

// BackupTo writes a consistent, standalone snapshot of the database to path
// using SQLite's "VACUUM INTO". Unlike copying the database file, this is safe
// on a live DB in WAL mode: it runs a read transaction and produces a fully
// checkpointed, self-contained copy (no accompanying -wal/-shm needed).
//
// path must not already exist — VACUUM INTO refuses to overwrite — and must be
// on a LOCAL filesystem: VACUUM INTO creates a real SQLite database and takes
// file locks on it, which are unreliable on network mounts (NFS/CIFS) and fail
// with SQLITE_BUSY. Callers writing to a network share should snapshot locally
// and then copy the finished file across. Because the store uses a single
// connection, this briefly serializes with other queries for its duration.
func (s *Store) BackupTo(path string) error {
	_, err := s.db.Exec(`VACUUM INTO ?`, path)
	return err
}
