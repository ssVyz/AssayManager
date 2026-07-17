// Package config holds runtime configuration, sourced from flags with
// environment-variable fallbacks. Kept deliberately small for the MVP.
package config

import (
	"flag"
	"os"
)

type Config struct {
	Addr           string // HTTP listen address
	DBPath         string // path to the SQLite database file
	LogPath        string // path to the append-only log file
	InclusivityBin string // path to the analysis binary (analysis is stubbed in v1)
	MaxUploadBytes int64  // cap on pasted/imported assay payloads
}

// Load parses flags (with env fallbacks) and returns the configuration.
func Load() Config {
	c := Config{
		Addr:           envOr("AM_ADDR", ":8080"),
		DBPath:         envOr("AM_DB", "assaymanager.db"),
		LogPath:        envOr("AM_LOG", "assaymanager.log"),
		InclusivityBin: envOr("AM_INCLUSIVITY_BIN", "./assets/inclusivity_check_blast"),
		MaxUploadBytes: 1 << 20, // 1 MiB
	}
	flag.StringVar(&c.Addr, "addr", c.Addr, "HTTP listen address")
	flag.StringVar(&c.DBPath, "db", c.DBPath, "path to the SQLite database file")
	flag.StringVar(&c.LogPath, "log", c.LogPath, "path to the append-only log file")
	flag.StringVar(&c.InclusivityBin, "inclusivity-bin", c.InclusivityBin,
		"path to the inclusivity_check_blast binary (analysis is stubbed in v1)")
	flag.Parse()
	return c
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
