package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadOrCreateBackupConfigGeneratesDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backup.ini")

	cfg, created, err := LoadOrCreateBackupConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatalf("expected created=true for a missing file")
	}
	if cfg.Enabled {
		t.Errorf("default must be disabled")
	}
	if cfg.Dir != defaultBackupDir {
		t.Errorf("default dir = %q, want %q", cfg.Dir, defaultBackupDir)
	}
	if cfg.Interval != defaultBackupInterval {
		t.Errorf("default interval = %v, want %v", cfg.Interval, defaultBackupInterval)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("template file was not written: %v", err)
	}

	// A second load reads the file and does not recreate it.
	if _, created2, err := LoadOrCreateBackupConfig(path); err != nil || created2 {
		t.Errorf("second load = (created=%v, err=%v), want (false, nil)", created2, err)
	}
}

func TestLoadBackupConfigParsesValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backup.ini")
	content := "# a comment\nenabled = true\ndir = /mnt/backups\ninterval = 6h\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, created, err := LoadOrCreateBackupConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Errorf("existing file must not be recreated")
	}
	if !cfg.Enabled {
		t.Errorf("enabled should be true")
	}
	if cfg.Dir != "/mnt/backups" {
		t.Errorf("dir = %q, want /mnt/backups", cfg.Dir)
	}
	if cfg.Interval != 6*time.Hour {
		t.Errorf("interval = %v, want 6h", cfg.Interval)
	}
}

func TestLoadBackupConfigReportsBadValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backup.ini")
	if err := os.WriteFile(path, []byte("enabled = maybe\ninterval = soon\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, _, err := LoadOrCreateBackupConfig(path)
	if err == nil {
		t.Errorf("expected an error reporting the bad values")
	}
	// Bad values fall back to safe defaults so startup can proceed.
	if cfg.Enabled {
		t.Errorf("bad enabled must fall back to false")
	}
	if cfg.Interval != defaultBackupInterval {
		t.Errorf("bad interval must fall back to %v", defaultBackupInterval)
	}
}
