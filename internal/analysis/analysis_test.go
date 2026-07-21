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
		"--blast-query":        "ACGTACGTACGT",
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
