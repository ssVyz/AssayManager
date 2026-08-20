package backup

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"AssayManager/internal/config"
	"AssayManager/internal/store"
)

func newTestRunner(t *testing.T, dir string, interval time.Duration) (*Runner, *store.Store) {
	t.Helper()
	// The DB lives in its own local temp dir, which is also used as the runner's
	// local scratch space for snapshots (kept off the "destination" dir).
	dbPath := filepath.Join(t.TempDir(), "am.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser("u", "h"); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := New(config.BackupConfig{Enabled: true, Dir: dir, Interval: interval}, dbPath, st, log)
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

	// The published file must be a usable, standalone database (proves the full
	// snapshot → copy → rename pipeline produced a valid artifact), and no temp
	// files should be left behind on the destination.
	produced := onlyDBFile(t, dir)
	b, err := store.Open(produced)
	if err != nil {
		t.Fatalf("produced backup does not open: %v", err)
	}
	if _, err := b.UserByID(1); err != nil {
		t.Errorf("produced backup missing seeded user: %v", err)
	}
	b.Close()
	for _, e := range readDir(t, dir) {
		if strings.HasSuffix(e, ".part") || strings.HasSuffix(e, ".tmp") {
			t.Errorf("destination has leftover temp file %q", e)
		}
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

func readDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// onlyDBFile returns the path of the single ".db" file in dir, failing if there
// is not exactly one.
func onlyDBFile(t *testing.T, dir string) string {
	t.Helper()
	var found string
	for _, name := range readDir(t, dir) {
		if filepath.Ext(name) == ".db" {
			if found != "" {
				t.Fatalf("expected exactly one .db file in %s", dir)
			}
			found = filepath.Join(dir, name)
		}
	}
	if found == "" {
		t.Fatalf("no .db file in %s", dir)
	}
	return found
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
