package web

import (
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
    {"rank": 1, "signature": "0|0", "count": 6, "percentage": 60.0,
     "cumulative_percentage": 60.0, "total_mismatches": 0, "matched_fwd": 6,
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

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
