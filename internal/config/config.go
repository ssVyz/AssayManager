// Package config holds runtime configuration, sourced from flags with
// environment-variable fallbacks. Kept deliberately small for the MVP.
package config

import (
	"flag"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Addr           string // HTTP listen address
	DBPath         string // path to the SQLite database file
	LogPath        string // path to the append-only log file
	InclusivityBin string // path to the analysis binary

	MaxUploadBytes          int64         // cap on normal form bodies
	MaxReferenceUploadBytes int64         // cap on the reference FASTA upload
	AnalysisTimeout         time.Duration // per-run analysis timeout
	MaxConcurrentRuns       int           // bound on simultaneous analysis runs
}

// Load parses flags (with env fallbacks) and returns the configuration.
func Load() Config {
	c := Config{
		Addr:           envOr("AM_ADDR", ":8080"),
		DBPath:         envOr("AM_DB", "assaymanager.db"),
		LogPath:        envOr("AM_LOG", "assaymanager.log"),
		InclusivityBin: envOr("AM_INCLUSIVITY_BIN", "./assets/inclusivity_check_blast"),

		MaxUploadBytes:          1 << 20,                               // 1 MiB
		MaxReferenceUploadBytes: envBytes("AM_MAX_REF_UPLOAD", 50<<20), // 50 MiB
		AnalysisTimeout:         envDuration("AM_ANALYSIS_TIMEOUT", 30*time.Minute),
		MaxConcurrentRuns:       envInt("AM_MAX_CONCURRENT_RUNS", 2),
	}
	flag.StringVar(&c.Addr, "addr", c.Addr, "HTTP listen address")
	flag.StringVar(&c.DBPath, "db", c.DBPath, "path to the SQLite database file")
	flag.StringVar(&c.LogPath, "log", c.LogPath, "path to the append-only log file")
	flag.StringVar(&c.InclusivityBin, "inclusivity-bin", c.InclusivityBin,
		"path to the inclusivity_check_blast binary")
	flag.Parse()
	return c
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBytes(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return def
}
