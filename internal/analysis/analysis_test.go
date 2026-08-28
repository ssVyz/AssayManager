package analysis

import (
	"strings"
	"testing"
)

// argValue returns the argument following the given flag, or "" if absent.
func argValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func TestBuildArgsFileMode(t *testing.T) {
	c := &CLI{}
	args := c.buildArgs("/work", Request{ReferencePath: "/work/ref.fasta"})

	joined := strings.Join(args, " ")
	if !strings.HasSuffix(joined, "assay.json /work/ref.fasta") {
		t.Errorf("file mode should end with assay + reference positionals; got: %s", joined)
	}
	if hasFlag(args, "--ref-source") {
		t.Errorf("file mode must not set --ref-source; got: %s", joined)
	}
	for _, f := range []string{"--json", "--xlsx", "--txt", "--no-config", "-q"} {
		if !hasFlag(args, f) {
			t.Errorf("missing expected flag %s", f)
		}
	}
}

func TestBuildArgsBlastMode(t *testing.T) {
	c := &CLI{ncbiEmail: "me@lab.org", ncbiTool: "AssayManager"}
	args := c.buildArgs("/work", Request{
		Blast: &BlastParams{
			Query:       "ACGTACGTACGT",
			TaxIDs:      []int{1128, 562},
			From:        "2020/01/01",
			To:          "2024/12/31",
			MinCoverage: 0.9,
			MinIdentity: 0.6,
			HitlistSize: 20000,
		},
	})

	checks := map[string]string{
		"--ref-source":         "blast",
		"--blast-taxid":        "1128,562",
		"--blast-from":         "2020/01/01",
		"--blast-to":           "2024/12/31",
		"--blast-min-coverage": "0.9",
		"--blast-min-identity": "0.6",
		"--blast-hitlist-size": "20000",
		"--ncbi-email":         "me@lab.org",
		"--ncbi-tool":          "AssayManager",
	}
	for flag, want := range checks {
		if got := argValue(args, flag); got != want {
			t.Errorf("%s = %q, want %q", flag, got, want)
		}
	}
	// The query is passed as a file path (written by Run), not inline, so a long
	// sequence can't be misread as an over-long filename (OS-specific separator).
	if q := argValue(args, "--blast-query"); !strings.HasSuffix(q, "query.fasta") {
		t.Errorf("--blast-query = %q, want a path ending in query.fasta", q)
	}
	// BLAST mode must not attach a reference-file positional.
	if strings.Contains(strings.Join(args, " "), "ref.fasta") {
		t.Errorf("blast mode should not include a reference file positional")
	}
	// The assay must still be the trailing positional (OS-specific separator).
	if last := args[len(args)-1]; !strings.HasSuffix(last, "assay.json") {
		t.Errorf("last arg = %q, want the assay path", last)
	}
}

func TestBuildArgsBlastOmitsUnsetOptionals(t *testing.T) {
	c := &CLI{ncbiEmail: "me@lab.org"}
	args := c.buildArgs("/work", Request{
		Blast: &BlastParams{Query: "ACGT", TaxIDs: []int{1}},
	})
	for _, f := range []string{"--blast-from", "--blast-to", "--blast-min-coverage", "--blast-min-identity", "--blast-hitlist-size", "--ncbi-tool"} {
		if hasFlag(args, f) {
			t.Errorf("unset optional %s should be omitted", f)
		}
	}
}

// TestSignatureMismatches covers the per-oligo mismatch count derived from a
// signature cell: dots are matches, anything else (a base or a gap dash) is a
// mismatch, the orientation suffix is not part of the alignment, and an
// unmatched oligo reports no count at all.
func TestSignatureMismatches(t *testing.T) {
	cases := []struct {
		cell    string
		want    int
		matched bool
	}{
		{"........(fwd)", 0, true},
		{"......A.(rev)", 1, true},
		{"..A..T", 2, true},
		{".....-.", 1, true},
		{"NO_MATCH", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := signatureMismatches(c.cell)
		if ok != c.matched || (ok && got != c.want) {
			t.Errorf("signatureMismatches(%q) = %d, %v; want %d, %v", c.cell, got, ok, c.want, c.matched)
		}
	}
}

// TestTableClassCells checks the collapsed per-class pattern cells: the best
// oligo of a class wins (so one perfect oligo makes the class perfect) and a
// class with no matching oligo reads "none".
func TestTableClassCells(t *testing.T) {
	r := &Result{}
	r.Meta.SchemaVersion = SupportedSchemaVersion
	r.Meta.Oligos.ForwardPrimers = []OligoRef{{ID: "F1", Seq: "ACGTAC"}, {ID: "F2", Seq: "ACGTAC"}}
	r.Meta.Oligos.Probes = []OligoRef{{ID: "P1", Seq: "ACGTAC"}}
	r.Meta.Oligos.ReversePrimers = []OligoRef{{ID: "R1", Seq: "ACGTAC"}}
	r.Summary.TotalSequences = 10
	r.Patterns = []Pattern{
		// F1 has two mismatches but F2 is perfect; the probe has one; no reverse hit.
		{Rank: 1, Signature: "..A..T(fwd) | ......(fwd) || ...G..(fwd) || NO_MATCH", Count: 6},
		// Nothing matched in any class.
		{Rank: 2, Signature: "NO_MATCH | NO_MATCH || NO_MATCH || NO_MATCH", Count: 4},
	}

	v := r.Table()

	wantCols := []string{"Forward primer(s)", "Probe(s)", "Reverse primer(s)"}
	if strings.Join(v.ClassCols, "|") != strings.Join(wantCols, "|") {
		t.Fatalf("ClassCols = %v, want %v", v.ClassCols, wantCols)
	}
	wantCells := [][]string{
		{"perfect", "1 mm", "none"},
		{"none", "none", "none"},
	}
	for i, want := range wantCells {
		got := v.PatternRows[i].ClassCells
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("pattern %d ClassCells = %v, want %v", i+1, got, want)
		}
		// The full per-oligo projection stays available alongside the collapsed one.
		if len(v.PatternRows[i].Cells) != len(v.OligoCols) {
			t.Errorf("pattern %d: %d cells for %d oligo columns", i+1, len(v.PatternRows[i].Cells), len(v.OligoCols))
		}
	}
}

// TestTableClassColsWithoutProbes omits the probe column for a probe-less assay,
// keeping ClassCols and ClassCells aligned.
func TestTableClassColsWithoutProbes(t *testing.T) {
	r := &Result{}
	r.Meta.Oligos.ForwardPrimers = []OligoRef{{ID: "F1", Seq: "ACGTAC"}}
	r.Meta.Oligos.ReversePrimers = []OligoRef{{ID: "R1", Seq: "ACGTAC"}}
	r.Summary.TotalSequences = 1
	r.Patterns = []Pattern{{Rank: 1, Signature: "......(fwd) || .....T(rev)", Count: 1}}

	v := r.Table()
	if len(v.ClassCols) != 2 || v.ClassCols[1] != "Reverse primer(s)" {
		t.Fatalf("ClassCols = %v, want forward + reverse only", v.ClassCols)
	}
	if got := v.PatternRows[0].ClassCells; strings.Join(got, "|") != "perfect|1 mm" {
		t.Errorf("ClassCells = %v, want [perfect 1 mm]", got)
	}
}
