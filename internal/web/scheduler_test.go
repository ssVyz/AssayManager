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

// fakeAnalyzer stands in for the CLI so scheduler tests never touch NCBI.
type fakeAnalyzer struct{}

func (fakeAnalyzer) Name() string         { return "fake" }
func (fakeAnalyzer) Available() bool      { return true }
func (fakeAnalyzer) BlastAvailable() bool { return true }
func (fakeAnalyzer) Run(context.Context, analysis.Request) (analysis.Report, error) {
	return analysis.Report{RawJSON: []byte("{}"), ToolName: "fake", SchemaVersion: 1}, nil
}

func TestFireScheduleEnqueuesAndAdvances(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	uid, _ := st.CreateUser("u", "h")
	fwd, _ := assayparser.MkOligo("F", assayparser.FuncForwardPrimer, "ATGCATGCATGC")
	rev, _ := assayparser.MkOligo("R", assayparser.FuncReversePrimer, "TTCTAGGGTAGG")
	va := assayparser.ValidAssay{
		Header:  assayparser.AssayHeader{Name: "SchedAssay", Author: "t"},
		Oligos:  assayparser.AssayOligos{OligoList: []assayparser.Oligo{fwd, rev}},
		Targets: assayparser.AssayTargets{TgtTaxids: []int{123}, RefAmpliconSeq: "AATACTAATCGT"},
	}
	assay, _ := st.SaveNewVersion(uid, va, "minor")
	if _, err := st.CreateSchedule(uid, assay.ID, "blast", 12, 30, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv, err := New(config.Config{MaxConcurrentRuns: 1}, logger, st, auth.NewManager(time.Hour), fakeAnalyzer{})
	if err != nil {
		t.Fatal(err)
	}

	srv.scheduleTick(context.Background())
	time.Sleep(150 * time.Millisecond) // let the background run goroutine finish

	results, err := st.ListResults(uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("fired schedule should create exactly one run, got %d", len(results))
	}

	list, _ := st.ListSchedules(uid)
	if len(list) != 1 {
		t.Fatalf("expected 1 schedule, got %d", len(list))
	}
	if !list[0].NextExecution.After(time.Now()) {
		t.Errorf("next_execution was not advanced to the future: %v", list[0].NextExecution)
	}
}
