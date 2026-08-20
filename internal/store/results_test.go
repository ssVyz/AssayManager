package store

import "testing"

// TestFailStaleRuns verifies the startup reconciliation sweep: runs still marked
// "running" (orphans from a previous process) are failed, while finished runs
// are left untouched.
func TestFailStaleRuns(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	uid, err := st.CreateUser("u", "h")
	if err != nil {
		t.Fatal(err)
	}
	a := seedBlastAssay(t, st, uid, "StaleAssay")

	// Two runs left "running", plus one already completed that must not change.
	r1, _ := st.CreateRun(uid, a, NewRun{Source: "blast"})
	r2, _ := st.CreateRun(uid, a, NewRun{Source: "blast"})
	rDone, _ := st.CreateRun(uid, a, NewRun{Source: "blast"})
	if err := st.CompleteRun(rDone, "{}", "tool", "1", 1); err != nil {
		t.Fatal(err)
	}

	n, err := st.FailStaleRuns("interrupted by a server restart")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("FailStaleRuns reset %d rows, want 2", n)
	}

	for _, id := range []int64{r1, r2} {
		res, err := st.ResultByID(uid, id)
		if err != nil {
			t.Fatal(err)
		}
		if res.Status != StatusFailed {
			t.Errorf("run %d status = %q, want %q", id, res.Status, StatusFailed)
		}
		if res.Error == "" {
			t.Errorf("run %d should carry an error message after reconciliation", id)
		}
		if res.FinishedAt == nil {
			t.Errorf("run %d should have finished_at set", id)
		}
	}

	if res, _ := st.ResultByID(uid, rDone); res.Status != StatusDone {
		t.Errorf("completed run status = %q, want %q (must not be touched)", res.Status, StatusDone)
	}

	// A second sweep with nothing running resets nothing.
	if n, err := st.FailStaleRuns("x"); err != nil || n != 0 {
		t.Errorf("second sweep = (%d, %v), want (0, nil)", n, err)
	}
}
