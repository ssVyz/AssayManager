# Changelog

All notable changes to AssayManager are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

The authoritative version lives in `main.go` (the `Version` constant) and must
match the latest entry below. Every code change gets a patch bump and a new
entry here.

## [0.1.0] - 2026-07-16

### Added
- Initial repository scaffolding: `main.go` with the authoritative `Version`
  constant, `CLAUDE.md` with agent rules, and this changelog.
- Integrated the `internal/assayparser` package (assay definition parsing,
  validation, and JSON/YAML I/O) into the `AssayManager` module. Removed its
  nested `go.mod`/`go.sum` so it builds and imports as a normal component
  (`AssayManager/internal/assayparser`).
- Added dependency `gopkg.in/yaml.v3 v3.0.1` (used by the assayparser I/O).
