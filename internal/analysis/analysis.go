// Package analysis defines the seam for assay analysis (inclusivity checking).
//
// v1 ships a Stub only. The real implementation will shell out to the
// inclusivity_check_blast CLI (see reference_files/inclusivity_check_description.md):
// write the assay JSON, run the binary with --json/--outdir/--prefix, and read
// back the consolidated JSON. Keeping this behind an interface means that
// implementation can drop in without touching callers.
package analysis

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Oligo is a single oligo handed to the analyzer (id + clean sequence).
type Oligo struct {
	ID  string
	Seq string
}

// Request is the analyzer input: oligos grouped by role, plus free-form params.
type Request struct {
	Forward []Oligo
	Reverse []Oligo
	Probes  []Oligo
	Params  map[string]string
}

// Report is the analyzer output. For now a single rendered text blob; the real
// tool will additionally carry structured JSON.
type Report struct {
	Format  string
	Content string
}

// Analyzer runs an inclusivity analysis for a request.
type Analyzer interface {
	Name() string
	Run(ctx context.Context, req Request) (Report, error)
}

// Stub is a placeholder Analyzer. It simulates a short-running job and returns
// a human-readable summary of what it received, so the run/results flow can be
// exercised end-to-end without the real tool.
type Stub struct {
	// Delay simulates a long-running analysis; keep small for the MVP.
	Delay time.Duration
}

func (Stub) Name() string { return "inclusivity_check_blast (stub)" }

func (s Stub) Run(ctx context.Context, req Request) (Report, error) {
	delay := s.Delay
	if delay == 0 {
		delay = 2 * time.Second
	}
	select {
	case <-ctx.Done():
		return Report{}, ctx.Err()
	case <-time.After(delay):
	}

	var b strings.Builder
	fmt.Fprintln(&b, "INCLUSIVITY CHECK — STUB REPORT")
	fmt.Fprintln(&b, "===============================")
	fmt.Fprintln(&b, "This is a placeholder. The real inclusivity_check_blast tool")
	fmt.Fprintln(&b, "is not yet integrated; no sequences were actually analysed.")
	fmt.Fprintln(&b)
	writeOligos(&b, "Forward primers", req.Forward)
	writeOligos(&b, "Reverse primers", req.Reverse)
	writeOligos(&b, "Probes", req.Probes)
	if len(req.Params) > 0 {
		fmt.Fprintln(&b, "\nParameters:")
		for k, v := range req.Params {
			fmt.Fprintf(&b, "  %s = %s\n", k, v)
		}
	}
	return Report{Format: "text", Content: b.String()}, nil
}

func writeOligos(b *strings.Builder, title string, os []Oligo) {
	fmt.Fprintf(b, "%s (%d):\n", title, len(os))
	if len(os) == 0 {
		fmt.Fprintln(b, "  (none)")
		return
	}
	for _, o := range os {
		fmt.Fprintf(b, "  - %s: %s\n", o.ID, o.Seq)
	}
}
