// Package backup runs periodic, consistent snapshots of the SQLite database to
// a destination directory (typically a mounted network volume). It is an
// ops-level concern, kept separate from the user-facing scheduler: configuration
// lives in an INI file read once at startup, and the runner tolerates the
// destination being temporarily unavailable.
package backup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"AssayManager/internal/config"
	"AssayManager/internal/store"
)

// checkInterval is how often the runner wakes to see whether a backup is due.
// Backups themselves happen at the configured (much longer) interval; this is
// just the polling granularity, matching the analysis scheduler's cadence.
const checkInterval = time.Minute

// stampLayout formats the UTC timestamp embedded in each backup's filename. It
// is filesystem-safe and lexically sortable, and doubles as the record of when
// the backup was taken — the runner derives "next due" from the newest file's
// stamp, so no separate last-backup state can drift out of sync.
const stampLayout = "20060102T150405Z"

// Runner performs due backups on a ticker until its context is cancelled.
type Runner struct {
	cfg    config.BackupConfig
	prefix string // backup filename prefix, derived from the DB filename
	store  *store.Store
	log    *slog.Logger

	// available tracks the last-known reachability of the destination dir, so
	// unavailability is logged on transition rather than on every check.
	available bool
}

// New builds a Runner. dbPath is used only to derive the backup filename prefix
// (e.g. "assaymanager.db" → "assaymanager-<stamp>.db"), so multiple instances
// sharing one destination directory do not collide.
func New(cfg config.BackupConfig, dbPath string, st *store.Store, log *slog.Logger) *Runner {
	base := filepath.Base(dbPath)
	prefix := strings.TrimSuffix(base, filepath.Ext(base))
	if prefix == "" || prefix == "." {
		prefix = "backup"
	}
	return &Runner{cfg: cfg, prefix: prefix, store: st, log: log, available: true}
}

// Start launches the runner in the background until ctx is cancelled. It checks
// once immediately (so a missing destination is reported at startup and a first
// backup is taken promptly) and then on each tick.
func (r *Runner) Start(ctx context.Context) {
	go func() {
		r.checkAndBackup(time.Now())
		t := time.NewTicker(checkInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.checkAndBackup(time.Now())
			}
		}
	}()
}

// checkAndBackup takes a backup if one is due and the destination is reachable.
// It never creates the destination directory: doing so while a network volume
// is unmounted would silently write backups to local disk at the mountpoint.
func (r *Runner) checkAndBackup(now time.Time) {
	if !r.dirAvailable() {
		if r.available {
			r.log.Warn("backup directory unavailable; skipping backups until it returns (is the network volume mounted?)", "dir", r.cfg.Dir)
			r.available = false
		}
		return
	}
	if !r.available {
		r.log.Info("backup directory available again", "dir", r.cfg.Dir)
		r.available = true
	}

	newest, err := r.latestBackup()
	if err != nil {
		r.log.Error("scan backup directory", "dir", r.cfg.Dir, "err", err)
		return
	}
	if !newest.IsZero() && now.Sub(newest) < r.cfg.Interval {
		return // not due yet
	}
	r.backupNow(now)
}

// backupNow writes a snapshot to a temp file and atomically renames it into
// place, so a failed or partial backup never appears as a valid one. The
// outcome is appended to the backup log.
func (r *Runner) backupNow(now time.Time) {
	name := r.prefix + "-" + now.UTC().Format(stampLayout) + ".db"
	final := filepath.Join(r.cfg.Dir, name)
	tmp := final + ".tmp"

	_ = os.Remove(tmp) // VACUUM INTO refuses to overwrite an existing file
	if err := r.store.BackupTo(tmp); err != nil {
		_ = os.Remove(tmp)
		r.logResult(now, "error", name, 0, err)
		r.log.Error("backup failed", "file", final, "err", err)
		return
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		r.logResult(now, "error", name, 0, err)
		r.log.Error("backup rename failed", "file", final, "err", err)
		return
	}

	var size int64
	if fi, e := os.Stat(final); e == nil {
		size = fi.Size()
	}
	r.logResult(now, "ok", name, size, nil)
	r.log.Info("backup created", "file", final, "bytes", size)
}

// latestBackup returns the timestamp of the newest backup file in the
// destination directory, or the zero time if there are none.
func (r *Runner) latestBackup() (time.Time, error) {
	entries, err := os.ReadDir(r.cfg.Dir)
	if err != nil {
		return time.Time{}, err
	}
	var newest time.Time
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if ts, ok := r.parseStamp(e.Name()); ok && ts.After(newest) {
			newest = ts
		}
	}
	return newest, nil
}

// parseStamp extracts the timestamp from a backup filename ("<prefix>-<stamp>.db"),
// reporting false for anything that is not one of our backup files (e.g. the
// ".db.tmp" work files or the backup log).
func (r *Runner) parseStamp(name string) (time.Time, bool) {
	pre := r.prefix + "-"
	if !strings.HasPrefix(name, pre) || !strings.HasSuffix(name, ".db") {
		return time.Time{}, false
	}
	mid := name[len(pre) : len(name)-len(".db")]
	t, err := time.Parse(stampLayout, mid)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// logResult appends one tab-separated line to the backup log in the destination
// directory. A failure to write the log is itself logged but not fatal.
func (r *Runner) logResult(now time.Time, status, name string, size int64, cause error) {
	line := fmt.Sprintf("%s\t%s\t%s\t%d", now.UTC().Format(time.RFC3339), status, name, size)
	if cause != nil {
		line += "\t" + cause.Error()
	}
	line += "\n"

	logPath := filepath.Join(r.cfg.Dir, r.prefix+"-backup.log")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		r.log.Error("open backup log", "path", logPath, "err", err)
		return
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		r.log.Error("write backup log", "path", logPath, "err", err)
	}
}

func (r *Runner) dirAvailable() bool {
	info, err := os.Stat(r.cfg.Dir)
	return err == nil && info.IsDir()
}
