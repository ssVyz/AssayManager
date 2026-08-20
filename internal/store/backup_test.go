package store

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBackupToProducesUsableCopy proves VACUUM INTO writes a standalone,
// openable database with the same data, and refuses to overwrite.
func TestBackupToProducesUsableCopy(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "src.db"))
	if err != nil {
		t.Fatal(err)
	}
	uid, err := st.CreateUser("alice", "hash")
	if err != nil {
		t.Fatal(err)
	}
	seedBlastAssay(t, st, uid, "BackupAssay")

	dst := filepath.Join(dir, "snapshot.db")
	if err := st.BackupTo(dst); err != nil {
		t.Fatalf("BackupTo: %v", err)
	}

	// Refuses to overwrite an existing target.
	if err := st.BackupTo(dst); err == nil {
		t.Errorf("BackupTo to an existing path should fail")
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	if fi, err := os.Stat(dst); err != nil || fi.Size() == 0 {
		t.Fatalf("backup missing or empty: err=%v", err)
	}

	// The snapshot opens on its own and carries the data.
	b, err := Open(dst)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer b.Close()
	u, err := b.UserByID(uid)
	if err != nil {
		t.Fatalf("read user from backup: %v", err)
	}
	if u.Username != "alice" {
		t.Errorf("backup user = %q, want alice", u.Username)
	}
	if assays, err := b.ListAllAssays(uid); err != nil || len(assays) == 0 {
		t.Errorf("backup should contain the seeded assay: err=%v, n=%d", err, len(assays))
	}
}
