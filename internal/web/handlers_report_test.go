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
		">Forward primer(s)<",                                // class column heading
		">Reverse primer(s)<",                                //
		`<td class="cls">perfect</td>`,                       // forward: no mismatches
		`<td class="cls">1 mm</td>`,                          // reverse: one mismatch
		`<span class="on" aria-current="true">simple</span>`, // toggle marks the mode
		`href="/results/1?pattern=full"`,                     // ...and links to the other
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
		"ATGCATGC",                                         // the oligo sequence sub-heading
		`<span class="on" aria-current="true">full</span>`, // toggle marks the mode
		`<a href="/results/1">simple</a>`,                  // ...and links back
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
	if strings.Contains(simple, `class="patterntoggle`) {
		t.Errorf("report should not render the pattern toggle")
	}

	full := get("/results/report?id=" + itoa(runID) + "&pattern=full")
	if !strings.Contains(full, `<td class="mono">......A.(rev)</td>`) {
		t.Errorf("report with pattern=full should contain full signatures")
	}
}

// TestResultsListActionsAboveTable pins the results-list toolbar: the export and
// delete controls render above the table (so they stay reachable on a long list)
// while still being owned by the table's form via form="results-form" — that
// association is what carries the ticked ids and the CSRF token.
func TestResultsListActionsAboveTable(t *testing.T) {
	srv, cookie, _ := reportTestServer(t)
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

	body := get("/results")
	if !strings.Contains(body, `id="results-form"`) {
		t.Fatal("results list is missing the form id the actions refer to")
	}
	if n := strings.Count(body, `form="results-form"`); n != 3 {
		t.Errorf("%d controls associated with the results form, want 3 (pattern, export, delete)", n)
	}
	actions := strings.Index(body, `class="actions resultsactions"`)
	table := strings.Index(body, `<table class="grid"`)
	if actions < 0 || table < 0 || actions > table {
		t.Errorf("actions at %d, table at %d: actions must render above the table", actions, table)
	}
	// The filter row and the actions share one bar, above the list.
	if bar := strings.Index(body, `class="resultsbar"`); bar < 0 || bar > actions {
		t.Errorf("actions must sit inside the filter bar (bar at %d, actions at %d)", bar, actions)
	}

	// With nothing to act on there are no export/delete controls at all.
	empty := get("/results?assay=NoSuchAssay")
	if strings.Contains(empty, `form="results-form"`) {
		t.Error("empty results list should not render the export/delete actions")
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
