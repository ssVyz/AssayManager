package backup

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"AssayManager/internal/config"
	"AssayManager/internal/store"
)

func newTestRunner(t *testing.T, dir string, interval time.Duration) (*Runner, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "am.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser("u", "h"); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := New(config.BackupConfig{Enabled: true, Dir: dir, Interval: interval}, "am.db", st, log)
	return r, st
}

func countBackups(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".db" {
			n++
		}
	}
	return n
}

func TestRunnerCreatesBackupWhenDue(t *testing.T) {
	dir := t.TempDir()
	r, st := newTestRunner(t, dir, time.Hour)
	defer st.Close()

	now := time.Now()
	r.checkAndBackup(now) // none exist → due → one backup
	if got := countBackups(t, dir); got != 1 {
		t.Fatalf("after first check: %d backups, want 1", got)
	}

	r.checkAndBackup(now.Add(time.Minute)) // within interval → no new backup
	if got := countBackups(t, dir); got != 1 {
		t.Fatalf("within interval: %d backups, want 1", got)
	}

	r.checkAndBackup(now.Add(2 * time.Hour)) // past interval → new backup
	if got := countBackups(t, dir); got != 2 {
		t.Fatalf("past interval: %d backups, want 2", got)
	}

	// A backup log line should have been recorded for each backup.
	logPath := filepath.Join(dir, "am-backup.log")
	if b, err := os.ReadFile(logPath); err != nil {
		t.Errorf("backup log not written: %v", err)
	} else if got := countLines(b); got != 2 {
		t.Errorf("backup log has %d lines, want 2", got)
	}
}

func TestRunnerSkipsWhenDirMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "not-mounted")
	r, st := newTestRunner(t, missing, time.Hour)
	defer st.Close()

	r.checkAndBackup(time.Now()) // must not panic, must not create anything

	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Errorf("runner must not create the destination directory (got err=%v)", err)
	}
}

func countLines(b []byte) int {
	n := 0
	for _, c := range b {
		if c == '\n' {
			n++
		}
	}
	return n
}
