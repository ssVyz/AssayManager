package web

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"AssayManager/internal/assayparser"
	"AssayManager/internal/auth"
	"AssayManager/internal/config"
	"AssayManager/internal/store"
)

// a minimal but schema-valid structured report (schema_version 1) so the report
// renders the full detail view (summary tiles, distribution, pattern table).
const sampleReportJSON = `{
  "meta": {
    "tool": "fake", "version": "1", "schema_version": 1, "method": "blast",
    "oligos": {
      "forward_primers": [{"id": "F", "seq": "ATGCATGC"}],
      "probes": [],
      "reverse_primers": [{"id": "R", "seq": "TTCTAGGG"}]
    }
  },
  "summary": {
    "total_sequences": 10,
    "sequences_with_min_matches": 8,
    "sequences_with_valid_amplicon": 7,
    "sequences_failed_amplicon": 3,
    "mismatch_distribution": {
      "forward": {"zero_mm": 6, "one_mm": 2, "more_mm": 1, "no_match": 1},
      "probe":   {"zero_mm": 0, "one_mm": 0, "more_mm": 0, "no_match": 0},
      "reverse": {"zero_mm": 7, "one_mm": 1, "more_mm": 1, "no_match": 1}
    },
    "overall": {"all_perfect": 6, "max_one_mismatch": 2, "two_plus_mismatches": 1, "no_match": 1},
    "oligo_stats": []
  },
  "patterns": [
    {"rank": 1, "signature": "........(fwd) || ......A.(rev)", "count": 6, "percentage": 60.0,
     "cumulative_percentage": 60.0, "total_mismatches": 1, "matched_fwd": 6,
     "matched_rev": 6, "matched_probe": 0, "amplicon_length": 120, "member_ids": ["s1", "s2"]}
  ],
  "per_sequence": []
}`

// reportTestServer sets up a store with one completed, structured run and returns
// the server, an authenticating cookie, and the run id.
func reportTestServer(t *testing.T) (*Server, *http.Cookie, int64) {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	uid, _ := st.CreateUser("u", "h")
	fwd, _ := assayparser.MkOligo("F", assayparser.FuncForwardPrimer, "ATGCATGCATGC")
	rev, _ := assayparser.MkOligo("R", assayparser.FuncReversePrimer, "TTCTAGGGTAGG")
	va := assayparser.ValidAssay{
		Header:  assayparser.AssayHeader{Name: "ReportAssay", Author: "t"},
		Oligos:  assayparser.AssayOligos{OligoList: []assayparser.Oligo{fwd, rev}},
		Targets: assayparser.AssayTargets{TgtTaxids: []int{123}, RefAmpliconSeq: "AATACTAATCGT"},
	}
	assay, _ := st.SaveNewVersion(uid, va, "minor")
	runID, err := st.CreateRun(uid, assay, store.NewRun{Source: "blast"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CompleteRun(runID, sampleReportJSON, "fake", "1", 1); err != nil {
		t.Fatal(err)
	}

	mgr := auth.NewManager(time.Hour)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv, err := New(config.Config{MaxConcurrentRuns: 1}, logger, st, mgr, fakeAnalyzer{})
	if err != nil {
		t.Fatal(err)
	}
	sess := mgr.Create(uid)
	return srv, &http.Cookie{Name: cookieName, Value: sess.ID}, runID
}

// TestResultsReportRenders exercises the full PDF-report pipeline (handler +
// print layout + shared detail partial), catching any template execution error.
func TestResultsReportRenders(t *testing.T) {
	srv, cookie, runID := reportTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/results/report?id="+itoa(runID), nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		"printdoc",            // print layout in use (not the app chrome)
		"Assay check results", // cover title
		"Exported",            // export timestamp line
		"ReportAssay",         // the run appears on the cover and in detail
		"Mismatch patterns",   // structured detail rendered via the shared partial
		"Run #",               // per-result detail heading
	} {
		if !strings.Contains(body, want) {
			t.Errorf("report body missing %q", want)
		}
	}
}

// TestResultsReportNoSelection redirects when no runs are selected.
func TestResultsReportNoSelection(t *testing.T) {
	srv, cookie, _ := reportTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/results/report", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", w.Code)
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "report_none") {
		t.Errorf("redirect = %q, want it to carry report_none", loc)
	}
}

// TestResultViewPatternSimpleDefault checks the single-result view defaults to
// the collapsed pattern table: one verdict per oligo class, no per-oligo columns.
func TestResultViewPatternSimpleDefault(t *testing.T) {
	body := getBody(t, "/results/")

	for _, want := range []string{
		"Mismatch patterns",
		">Forward primer(s)<",              // class column heading
		">Reverse primer(s)<",              //
		`<td class="cls">perfect</td>`,     // forward: no mismatches
		`<td class="cls">1 mm</td>`,        // reverse: one mismatch
		`<option value="simple" selected>`, // the picker reflects the mode
	} {
		if !strings.Contains(body, want) {
			t.Errorf("simple pattern view missing %q", want)
		}
	}
	// The pattern table shows no signature cells or per-oligo sub-headings; the
	// signature itself still appears in the (collapsed) raw-JSON block.
	for _, unwanted := range []string{
		`<td class="mono">`,
		`<span class="muted mono small">`,
	} {
		if strings.Contains(body, unwanted) {
			t.Errorf("simple pattern view should not contain %q", unwanted)
		}
	}
}

// TestResultViewPatternFull renders the unabridged per-oligo pattern table when
// ?pattern=full is requested.
func TestResultViewPatternFull(t *testing.T) {
	body := getBody(t, "/results/%s?pattern=full")

	for _, want := range []string{
		`<td class="mono">........(fwd)</td>`, // full signature per oligo
		`<td class="mono">......A.(rev)</td>`,
		"ATGCATGC",                       // the oligo sequence sub-heading
		`<option value="full" selected>`, // the picker reflects the mode
	} {
		if !strings.Contains(body, want) {
			t.Errorf("full pattern view missing %q", want)
		}
	}
	if strings.Contains(body, `class="cls"`) {
		t.Errorf("full pattern view should not contain collapsed class cells")
	}
}

// TestResultsReportPatternMode carries the pattern-detail choice into the
// printable report, which defaults to the collapsed view like the web page.
func TestResultsReportPatternMode(t *testing.T) {
	srv, cookie, runID := reportTestServer(t)
	get := func(url string) string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, url, nil)
		req.AddCookie(cookie)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", url, w.Code)
		}
		return w.Body.String()
	}

	simple := get("/results/report?id=" + itoa(runID))
	if !strings.Contains(simple, `<td class="cls">perfect</td>`) {
		t.Errorf("report should default to the collapsed pattern table")
	}
	if strings.Contains(simple, `<td class="mono">`) {
		t.Errorf("collapsed report should not contain signature cells")
	}
	// The report has no picker of its own; the mode comes from the export URL.
	if strings.Contains(simple, `class="patternpick`) {
		t.Errorf("report should not render the pattern picker")
	}

	full := get("/results/report?id=" + itoa(runID) + "&pattern=full")
	if !strings.Contains(full, `<td class="mono">......A.(rev)</td>`) {
		t.Errorf("report with pattern=full should contain full signatures")
	}
}

// getBody requests a single-result URL (with a %s placeholder for the run id, or
// a plain "/results/" prefix) and returns the rendered body.
func getBody(t *testing.T, urlFmt string) string {
	t.Helper()
	srv, cookie, runID := reportTestServer(t)
	url := "/results/" + itoa(runID)
	if strings.Contains(urlFmt, "%s") {
		url = fmt.Sprintf(urlFmt, itoa(runID))
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200; body: %s", url, w.Code, w.Body.String())
	}
	return w.Body.String()
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
