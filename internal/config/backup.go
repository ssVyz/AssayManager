package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Backup defaults, also reflected in the generated template.
const (
	defaultBackupDir      = "./backups"
	defaultBackupInterval = 24 * time.Hour
)

// BackupConfig is the ops-level backup configuration, read once at startup from
// a small INI-style file kept beside the database (not exposed in the web UI).
type BackupConfig struct {
	Enabled  bool
	Dir      string        // destination directory; must already exist (not created)
	Interval time.Duration // time between backups
}

// backupConfigTemplate is written verbatim when no config file exists. It is
// self-documenting so an admin can edit it without external reference.
const backupConfigTemplate = `# AssayManager backup configuration.
# Read once at server startup - edit and restart to apply changes.
#
# Backups use SQLite's "VACUUM INTO" to write a consistent, standalone database
# file. To restore: stop the server, replace the live database with a chosen
# backup file, delete any stale -wal/-shm files next to it, then start again.

# Turn backups on. Off by default so nothing is written until a destination is
# chosen below.
enabled = false

# Destination directory for backup files and the backup log. It must ALREADY
# EXIST - it is never created automatically. This is typically a mounted network
# volume; if it is missing at startup or backup time (e.g. the volume is not
# mounted), the server logs a warning and skips the backup, retrying on the next
# check. Backups are never deleted automatically; prune old files manually.
dir = ./backups

# Time between backups, as a Go duration string, e.g. 6h, 24h, 168h (1 week).
interval = 24h
`

// LoadOrCreateBackupConfig reads the backup config at path. If the file does not
// exist it writes the default template and returns the defaults with
// created=true. Unknown keys are ignored; malformed values fall back to a safe
// default and are reported via the returned error (startup should log it and
// carry on rather than abort).
func LoadOrCreateBackupConfig(path string) (cfg BackupConfig, created bool, err error) {
	cfg = BackupConfig{Enabled: false, Dir: defaultBackupDir, Interval: defaultBackupInterval}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			if werr := os.WriteFile(path, []byte(backupConfigTemplate), 0o644); werr != nil {
				return cfg, false, fmt.Errorf("write default backup config: %w", werr)
			}
			return cfg, true, nil
		}
		return cfg, false, err
	}
	defer f.Close()

	var probs []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimRight(sc.Text(), "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:eq]))
		val := unquote(strings.TrimSpace(line[eq+1:]))
		switch key {
		case "enabled":
			if b, e := strconv.ParseBool(val); e == nil {
				cfg.Enabled = b
			} else {
				probs = append(probs, fmt.Sprintf("invalid enabled %q", val))
			}
		case "dir":
			if val != "" {
				cfg.Dir = val
			}
		case "interval":
			if d, e := time.ParseDuration(val); e == nil && d > 0 {
				cfg.Interval = d
			} else {
				probs = append(probs, fmt.Sprintf("invalid interval %q", val))
			}
		}
	}
	if serr := sc.Err(); serr != nil {
		return cfg, false, serr
	}
	if len(probs) > 0 {
		return cfg, false, fmt.Errorf("backup config: %s", strings.Join(probs, "; "))
	}
	return cfg, false, nil
}
