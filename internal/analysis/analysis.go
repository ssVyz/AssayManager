// Package analysis integrates the inclusivity_check_blast CLI as a subprocess.
//
// AssayManager writes the assay (in its own JSON format, which the tool parses
// directly) to a temp file, runs the binary with --emit-json-stdout against an
// uploaded reference FASTA, and reads the consolidated JSON back from stdout.
// The Analyzer interface keeps this behind a seam so callers don't shell out
// directly (and so a future in-process implementation could replace it).
package analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// SupportedSchemaVersion is the inclusivity_check_blast JSON schema this build
// understands. Assert on schema_version, never on the tool's version.
const SupportedSchemaVersion = 1

// Request is the analyzer input: the assay as AssayManager-format JSON and a
// path to a reference FASTA already written to disk.
type Request struct {
	AssayJSON     []byte
	ReferencePath string
}

// Report is the analyzer output: the tool's raw consolidated JSON plus the meta
// fields pulled out for storage.
type Report struct {
	RawJSON       []byte
	ToolName      string
	ToolVersion   string
	SchemaVersion int
}

// Analyzer runs an inclusivity analysis for a request.
type Analyzer interface {
	Name() string
	// Available reports whether the analyzer can actually run (binary present
	// and compatible). The web layer uses this to enable/disable the run UI.
	Available() bool
	Run(ctx context.Context, req Request) (Report, error)
}

// ----------------------------------------------------------------------------
// Consolidated-JSON parse structs (subset; see the tool's report/json.go)
// ----------------------------------------------------------------------------

type Result struct {
	Meta        ResultMeta    `json:"meta"`
	Summary     ResultSummary `json:"summary"`
	Patterns    []Pattern     `json:"patterns"`
	PerSequence []PerSeq      `json:"per_sequence"`
}

type ResultMeta struct {
	Tool          string `json:"tool"`
	Version       string `json:"version"`
	SchemaVersion int    `json:"schema_version"`
	Method        string `json:"method"`
}

type ResultSummary struct {
	TotalSequences             int `json:"total_sequences"`
	SequencesWithMinMatches    int `json:"sequences_with_min_matches"`
	SequencesWithValidAmplicon int `json:"sequences_with_valid_amplicon"`
	SequencesFailedAmplicon    int `json:"sequences_failed_amplicon"`
	Overall                    struct {
		AllPerfect        int `json:"all_perfect"`
		MaxOneMismatch    int `json:"max_one_mismatch"`
		TwoPlusMismatches int `json:"two_plus_mismatches"`
		NoMatch           int `json:"no_match"`
	} `json:"overall"`
	OligoStats []OligoStat `json:"oligo_stats"`
}

type OligoStat struct {
	ID               string `json:"id"`
	Category         string `json:"category"`
	TotalMatches     int    `json:"total_matches"`
	SenseMatches     int    `json:"sense_matches"`
	AntisenseMatches int    `json:"antisense_matches"`
}

type Pattern struct {
	Rank                 int      `json:"rank"`
	Signature            string   `json:"signature"`
	Count                int      `json:"count"`
	Percentage           float64  `json:"percentage"`
	CumulativePercentage float64  `json:"cumulative_percentage"`
	TotalMismatches      int      `json:"total_mismatches"`
	MatchedFwd           int      `json:"matched_fwd"`
	MatchedRev           int      `json:"matched_rev"`
	MatchedProbe         int      `json:"matched_probe"`
	AmpliconLength       *int     `json:"amplicon_length"`
	MemberIDs            []string `json:"member_ids"`
}

type PerSeq struct {
	SeqID           string `json:"seq_id"`
	FwdMatched      int    `json:"fwd_matched"`
	ProbeMatched    int    `json:"probe_matched"`
	RevMatched      int    `json:"rev_matched"`
	TotalMismatches int    `json:"total_mismatches"`
	AmpliconFound   bool   `json:"amplicon_found"`
	AmpliconSize    *int   `json:"amplicon_size"`
	MeetsThresholds bool   `json:"meets_thresholds"`
}

// ParseResult decodes the tool's consolidated JSON and checks the schema version.
func ParseResult(raw []byte) (*Result, error) {
	var r Result
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	if r.Meta.SchemaVersion != SupportedSchemaVersion {
		return nil, fmt.Errorf("unsupported result schema_version %d (want %d)", r.Meta.SchemaVersion, SupportedSchemaVersion)
	}
	return &r, nil
}

// ----------------------------------------------------------------------------
// CLI subprocess analyzer
// ----------------------------------------------------------------------------

type capabilities struct {
	Tool          string   `json:"tool"`
	Version       string   `json:"version"`
	SchemaVersion int      `json:"schema_version"`
	Methods       []string `json:"methods"`
	Flags         []string `json:"flags"`
}

// CLI runs the inclusivity_check_blast binary as a subprocess.
type CLI struct {
	binPath   string
	timeout   time.Duration
	log       *slog.Logger
	available bool
	caps      capabilities
}

// NewCLI resolves the binary, runs a --capabilities health check, and returns a
// CLI. If the binary is missing or its schema is incompatible, the returned CLI
// reports Available() == false (the caller then disables the run feature). It
// never returns an error — analysis is an optional feature.
func NewCLI(binPath string, timeout time.Duration, log *slog.Logger) *CLI {
	c := &CLI{timeout: timeout, log: log}

	resolved, ok := resolveBinary(binPath)
	if !ok {
		log.Warn("analysis tool not found; analysis disabled", "path", binPath)
		return c
	}
	c.binPath = resolved

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, resolved, "--capabilities").Output()
	if err != nil {
		log.Warn("analysis capabilities check failed; analysis disabled", "path", resolved, "err", err)
		return c
	}
	if err := json.Unmarshal(out, &c.caps); err != nil {
		log.Warn("analysis capabilities not parseable; analysis disabled", "err", err)
		return c
	}
	if c.caps.SchemaVersion != SupportedSchemaVersion {
		log.Warn("analysis tool schema mismatch; analysis disabled",
			"tool_schema", c.caps.SchemaVersion, "supported", SupportedSchemaVersion)
		return c
	}
	c.available = true
	log.Info("analysis tool ready",
		"tool", c.caps.Tool, "version", c.caps.Version, "schema", c.caps.SchemaVersion, "path", resolved)
	return c
}

func (c *CLI) Name() string {
	if c.caps.Tool != "" {
		return c.caps.Tool + " " + c.caps.Version
	}
	return "inclusivity_check_blast"
}

func (c *CLI) Available() bool { return c.available }

// Run writes the assay to a temp working dir and invokes the binary against the
// reference FASTA, capturing the consolidated JSON from stdout. The working dir
// is removed afterwards; the reference file is the caller's to clean up.
func (c *CLI) Run(ctx context.Context, req Request) (Report, error) {
	if !c.available {
		return Report{}, errors.New("analysis tool is not available")
	}

	dir, err := os.MkdirTemp("", "am-run-*")
	if err != nil {
		return Report{}, fmt.Errorf("create work dir: %w", err)
	}
	defer os.RemoveAll(dir)

	assayPath := filepath.Join(dir, "assay.json")
	if err := os.WriteFile(assayPath, req.AssayJSON, 0o600); err != nil {
		return Report{}, fmt.Errorf("write assay: %w", err)
	}

	runCtx := ctx
	if c.timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(runCtx, c.binPath,
		"--emit-json-stdout", "--no-config", "-q",
		assayPath, req.ReferencePath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		switch {
		case errors.Is(runCtx.Err(), context.DeadlineExceeded):
			return Report{}, fmt.Errorf("analysis timed out after %s", c.timeout)
		case errors.Is(ctx.Err(), context.Canceled):
			return Report{}, ctx.Err()
		default:
			msg := lastLine(stderr.String())
			if msg == "" {
				msg = err.Error()
			}
			return Report{}, fmt.Errorf("analysis failed: %s", msg)
		}
	}

	raw := bytes.TrimSpace(stdout.Bytes())
	rep := Report{RawJSON: raw}
	if res, perr := ParseResult(raw); perr != nil {
		return Report{}, fmt.Errorf("analysis produced unreadable output: %w", perr)
	} else {
		rep.ToolName = res.Meta.Tool
		rep.ToolVersion = res.Meta.Version
		rep.SchemaVersion = res.Meta.SchemaVersion
	}
	return rep, nil
}

// resolveBinary returns the usable binary path. It accepts the configured path
// as-is, and on Windows falls back to the ".exe" variant (dev convenience).
func resolveBinary(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	if isFile(path) {
		return path, true
	}
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(path), ".exe") {
		if isFile(path + ".exe") {
			return path + ".exe", true
		}
	}
	return "", false
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
