// Package analysis integrates the inclusivity_check_blast CLI as a subprocess.
//
// AssayManager writes the assay (in its own JSON format, which the tool parses
// directly) to a temp file, runs the binary against an uploaded reference FASTA,
// and reads back the consolidated JSON plus the formatted report files (xlsx,
// txt) which are stored with the result for download. The Analyzer interface
// keeps this behind a seam.
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
	"strconv"
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

// Artifact is one generated output file (e.g. the xlsx workbook) captured for
// later download.
type Artifact struct {
	Kind    string // "xlsx", "txt"
	Content []byte
}

// Report is the analyzer output: the tool's raw consolidated JSON, the captured
// output files, and the meta fields pulled out for storage.
type Report struct {
	RawJSON       []byte
	Artifacts     []Artifact
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
	Oligos        struct {
		ForwardPrimers []OligoRef `json:"forward_primers"`
		Probes         []OligoRef `json:"probes"`
		ReversePrimers []OligoRef `json:"reverse_primers"`
	} `json:"oligos"`
}

type OligoRef struct {
	ID  string `json:"id"`
	Seq string `json:"seq"`
}

type MismatchDist struct {
	ZeroMm  int `json:"zero_mm"`
	OneMm   int `json:"one_mm"`
	MoreMm  int `json:"more_mm"`
	NoMatch int `json:"no_match"`
}

type ResultSummary struct {
	TotalSequences             int `json:"total_sequences"`
	SequencesWithMinMatches    int `json:"sequences_with_min_matches"`
	SequencesWithValidAmplicon int `json:"sequences_with_valid_amplicon"`
	SequencesFailedAmplicon    int `json:"sequences_failed_amplicon"`
	MismatchDistribution       struct {
		Forward MismatchDist `json:"forward"`
		Probe   MismatchDist `json:"probe"`
		Reverse MismatchDist `json:"reverse"`
	} `json:"mismatch_distribution"`
	Overall struct {
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
// Display view-model (mirrors the tool's Excel layout)
// ----------------------------------------------------------------------------

type OligoCol struct {
	ID       string
	Seq      string
	Category string // "Forward" | "Probe" | "Reverse"
}

type PatternRow struct {
	Num             int
	Cells           []string // one per OligoCol (the per-oligo signature)
	Count           int
	Percentage      float64
	Cumulative      float64
	TotalMismatches int
	MatchedFwd      int
	MatchedRev      int
	MatchedProbe    int
	AmpliconLength  string // "" if none
	Examples        string
}

type DistCell struct {
	Count int
	Pct   float64
}

type DistRow struct {
	Label string
	Zero  DistCell
	One   DistCell
	More  DistCell
	None  DistCell
}

type OverallRow struct {
	Label string
	DistCell
}

// ResultView is a display-ready projection of a Result: the per-oligo pattern
// table, per-class mismatch distribution (as percentages), and the overall
// breakdown — matching the tool's Excel output.
type ResultView struct {
	Total          int
	MeetThresholds int
	ValidAmplicon  int
	FailedAmplicon int
	OligoCols      []OligoCol
	PatternRows    []PatternRow
	ClassDist      []DistRow
	Overall        []OverallRow
}

// Table builds the display view-model from a parsed Result.
func (r *Result) Table() ResultView {
	total := r.Summary.TotalSequences

	var cols []OligoCol
	for _, o := range r.Meta.Oligos.ForwardPrimers {
		cols = append(cols, OligoCol{ID: o.ID, Seq: o.Seq, Category: "Forward"})
	}
	for _, o := range r.Meta.Oligos.Probes {
		cols = append(cols, OligoCol{ID: o.ID, Seq: o.Seq, Category: "Probe"})
	}
	for _, o := range r.Meta.Oligos.ReversePrimers {
		cols = append(cols, OligoCol{ID: o.ID, Seq: o.Seq, Category: "Reverse"})
	}
	nf := len(r.Meta.Oligos.ForwardPrimers)
	np := len(r.Meta.Oligos.Probes)
	nr := len(r.Meta.Oligos.ReversePrimers)

	rows := make([]PatternRow, 0, len(r.Patterns))
	for _, p := range r.Patterns {
		amp := ""
		if p.AmpliconLength != nil {
			amp = strconv.Itoa(*p.AmpliconLength)
		}
		rows = append(rows, PatternRow{
			Num:             p.Rank,
			Cells:           splitSignature(p.Signature, nf, np, nr),
			Count:           p.Count,
			Percentage:      p.Percentage,
			Cumulative:      p.CumulativePercentage,
			TotalMismatches: p.TotalMismatches,
			MatchedFwd:      p.MatchedFwd,
			MatchedRev:      p.MatchedRev,
			MatchedProbe:    p.MatchedProbe,
			AmpliconLength:  amp,
			Examples:        exampleIDs(p.MemberIDs),
		})
	}

	md := r.Summary.MismatchDistribution
	classDist := []DistRow{distRow("Forward primers", md.Forward, total)}
	if np > 0 {
		classDist = append(classDist, distRow("Probes", md.Probe, total))
	}
	classDist = append(classDist, distRow("Reverse primers", md.Reverse, total))

	ov := r.Summary.Overall
	overall := []OverallRow{
		{"All categories 0 mismatches", cell(ov.AllPerfect, total)},
		{"All categories ≤1 mismatch", cell(ov.MaxOneMismatch, total)},
		{"≥2 mismatches in any category", cell(ov.TwoPlusMismatches, total)},
		{"No match in any category", cell(ov.NoMatch, total)},
	}

	return ResultView{
		Total:          total,
		MeetThresholds: r.Summary.SequencesWithMinMatches,
		ValidAmplicon:  r.Summary.SequencesWithValidAmplicon,
		FailedAmplicon: r.Summary.SequencesFailedAmplicon,
		OligoCols:      cols,
		PatternRows:    rows,
		ClassDist:      classDist,
		Overall:        overall,
	}
}

func distRow(label string, d MismatchDist, total int) DistRow {
	return DistRow{
		Label: label,
		Zero:  cell(d.ZeroMm, total),
		One:   cell(d.OneMm, total),
		More:  cell(d.MoreMm, total),
		None:  cell(d.NoMatch, total),
	}
}

func cell(count, total int) DistCell {
	return DistCell{Count: count, Pct: pct(count, total)}
}

func pct(count, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(count) / float64(total) * 100.0
}

func exampleIDs(ids []string) string {
	n := len(ids)
	if n == 0 {
		return ""
	}
	show := ids
	if n > 3 {
		show = ids[:3]
	}
	out := strings.Join(show, ", ")
	if n > 3 {
		out += fmt.Sprintf(" (+%d more)", n-3)
	}
	return out
}

// splitSignature splits a combined signature string into one cell per oligo
// (forward, probe, reverse order), padding missing entries with NO_MATCH. Ported
// from the tool's Excel reporter so the app's table matches it exactly.
func splitSignature(signature string, numFwd, numProbes, numRev int) []string {
	sections := strings.Split(signature, " || ")
	var all []string

	take := func(section string, n int) {
		var parts []string
		if section != "" {
			parts = strings.Split(section, " | ")
		}
		for i := 0; i < n; i++ {
			if i < len(parts) {
				all = append(all, parts[i])
			} else {
				all = append(all, "NO_MATCH")
			}
		}
	}

	fwdSection := ""
	if len(sections) >= 1 {
		fwdSection = sections[0]
	}
	take(fwdSection, numFwd)

	if numProbes > 0 {
		probeSection := ""
		if len(sections) >= 3 {
			probeSection = sections[1]
		}
		take(probeSection, numProbes)
	}

	revIdx := 1
	if numProbes > 0 {
		revIdx = 2
	}
	revSection := ""
	if len(sections) > revIdx {
		revSection = sections[revIdx]
	}
	take(revSection, numRev)

	return all
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
// reference FASTA, requesting the consolidated JSON plus the xlsx and txt report
// files. It returns the JSON and the captured files. The working dir is removed
// afterwards; the reference file is the caller's to clean up.
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
		"--json", "--xlsx", "--txt", "--no-config", "-q",
		"--outdir", dir, "--prefix", "result",
		assayPath, req.ReferencePath)
	var stderr bytes.Buffer
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

	base := filepath.Join(dir, "result")
	raw, err := os.ReadFile(base + ".json")
	if err != nil {
		return Report{}, fmt.Errorf("read result JSON: %w", err)
	}
	raw = bytes.TrimSpace(raw)

	res, perr := ParseResult(raw)
	if perr != nil {
		return Report{}, fmt.Errorf("analysis produced unreadable output: %w", perr)
	}

	rep := Report{
		RawJSON:       raw,
		ToolName:      res.Meta.Tool,
		ToolVersion:   res.Meta.Version,
		SchemaVersion: res.Meta.SchemaVersion,
	}
	for _, a := range []struct{ kind, ext string }{{"xlsx", ".xlsx"}, {"txt", ".txt"}} {
		if b, e := os.ReadFile(base + a.ext); e == nil {
			rep.Artifacts = append(rep.Artifacts, Artifact{Kind: a.kind, Content: b})
		} else {
			c.log.Warn("analysis output missing", "kind", a.kind, "err", e)
		}
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
