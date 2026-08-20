package web

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"AssayManager/internal/analysis"
	"AssayManager/internal/assayparser"
	"AssayManager/internal/auth"
	"AssayManager/internal/config"
	"AssayManager/internal/store"
)

// hangAnalyzer.Run blocks forever, ignoring ctx, to simulate a run goroutine
// that wedges and never returns (e.g. cmd.Wait stuck after a subprocess kill).
type hangAnalyzer struct{}

func (hangAnalyzer) Name() string         { return "hang" }
func (hangAnalyzer) Available() bool      { return true }
func (hangAnalyzer) BlastAvailable() bool { return true }
func (hangAnalyzer) Run(context.Context, analysis.Request) (analysis.Report, error) {
	select {} // never returns, never observes cancellation
}

// TestRunWatchdogFreesQueue asserts the watchdog fails a wedged run and releases
// the concurrency slot even though the run goroutine never returns.
func TestRunWatchdogFreesQueue(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	uid, _ := st.CreateUser("u", "h")
	fwd, _ := assayparser.MkOligo("F", assayparser.FuncForwardPrimer, "ATGCATGCATGC")
	rev, _ := assayparser.MkOligo("R", assayparser.FuncReversePrimer, "TTCTAGGGTAGG")
	va := assayparser.ValidAssay{
		Header:  assayparser.AssayHeader{Name: "HangAssay", Author: "t"},
		Oligos:  assayparser.AssayOligos{OligoList: []assayparser.Oligo{fwd, rev}},
		Targets: assayparser.AssayTargets{TgtTaxids: []int{123}, RefAmpliconSeq: "AATACTAATCGT"},
	}
	assay, _ := st.SaveNewVersion(uid, va, "minor")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv, err := New(config.Config{MaxConcurrentRuns: 1, AnalysisTimeout: 50 * time.Millisecond},
		logger, st, auth.NewManager(time.Hour), hangAnalyzer{})
	if err != nil {
		t.Fatal(err)
	}
	srv.watchdogGrace = 50 * time.Millisecond // fire fast in the test

	rid, err := st.CreateRun(uid, assay, store.NewRun{Source: "blast"})
	if err != nil {
		t.Fatal(err)
	}
	go srv.runAnalysis(rid, analysis.Request{}, "")

	// The watchdog (timeout + grace ≈ 100ms) should fail the row well inside 2s.
	deadline := time.Now().Add(2 * time.Second)
	for {
		res, err := st.ResultByID(uid, rid)
		if err != nil {
			t.Fatal(err)
		}
		if res.Status == store.StatusFailed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("watchdog did not fail the stuck run; status still %q", res.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// The slot must be free: the watchdog releases it before failing the row, so
	// a fresh acquire must succeed without blocking.
	select {
	case srv.runSem <- struct{}{}:
		<-srv.runSem
	default:
		t.Fatal("run slot was not released after the watchdog fired")
	}
}
