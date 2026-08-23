package store

import "testing"

// TestListResultsFiltered checks the assay-name and date-range narrowing, and
// that an empty filter is equivalent to listing everything.
func TestListResultsFiltered(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	uid, err := st.CreateUser("u", "h")
	if err != nil {
		t.Fatal(err)
	}
	alpha := seedBlastAssay(t, st, uid, "Alpha")
	beta := seedBlastAssay(t, st, uid, "Beta")

	st.CreateRun(uid, alpha, NewRun{Source: "blast"})
	st.CreateRun(uid, alpha, NewRun{Source: "blast"})
	st.CreateRun(uid, beta, NewRun{Source: "blast"})

	// No filter == all runs.
	all, err := st.ListResultsFiltered(uid, ResultFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("unfiltered = %d runs, want 3", len(all))
	}

	// Assay filter matches by name.
	onlyAlpha, err := st.ListResultsFiltered(uid, ResultFilter{AssayName: "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyAlpha) != 2 {
		t.Fatalf("assay=Alpha = %d runs, want 2", len(onlyAlpha))
	}
	for _, r := range onlyAlpha {
		if r.AssayName != "Alpha" {
			t.Errorf("assay filter leaked %q", r.AssayName)
		}
	}

	// Date lower bound in the far future excludes everything; the far past keeps
	// all. started_at is stamped at creation (~now), so these bracket it.
	if got, _ := st.ListResultsFiltered(uid, ResultFilter{FromUTC: "2999-01-01T00:00:00Z"}); len(got) != 0 {
		t.Errorf("from=2999 = %d runs, want 0", len(got))
	}
	if got, _ := st.ListResultsFiltered(uid, ResultFilter{FromUTC: "2000-01-01T00:00:00Z"}); len(got) != 3 {
		t.Errorf("from=2000 = %d runs, want 3", len(got))
	}
	// Upper bound is exclusive: a bound in the far past excludes all.
	if got, _ := st.ListResultsFiltered(uid, ResultFilter{ToUTC: "2000-01-01T00:00:00Z"}); len(got) != 0 {
		t.Errorf("to=2000 = %d runs, want 0", len(got))
	}
	if got, _ := st.ListResultsFiltered(uid, ResultFilter{ToUTC: "2999-01-01T00:00:00Z"}); len(got) != 3 {
		t.Errorf("to=2999 = %d runs, want 3", len(got))
	}
}

// TestResultAssayNames returns the distinct assay names present in a user's
// runs, sorted, and does not leak another user's data.
func TestResultAssayNames(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	uid, _ := st.CreateUser("u", "h")
	other, _ := st.CreateUser("o", "h")
	alpha := seedBlastAssay(t, st, uid, "Alpha")
	beta := seedBlastAssay(t, st, uid, "Beta")
	foreign := seedBlastAssay(t, st, other, "Foreign")

	st.CreateRun(uid, beta, NewRun{Source: "blast"})
	st.CreateRun(uid, alpha, NewRun{Source: "blast"})
	st.CreateRun(uid, alpha, NewRun{Source: "blast"}) // duplicate name
	st.CreateRun(other, foreign, NewRun{Source: "blast"})

	names, err := st.ResultAssayNames(uid)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Alpha", "Beta"}
	if len(names) != len(want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}

// TestDeleteResults verifies owner scoping, the returned count, and that a
// deleted run's stored artifacts are removed via the cascade.
func TestDeleteResults(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	uid, _ := st.CreateUser("u", "h")
	other, _ := st.CreateUser("o", "h")
	a := seedBlastAssay(t, st, uid, "Alpha")
	foreignAssay := seedBlastAssay(t, st, other, "Foreign")

	r1, _ := st.CreateRun(uid, a, NewRun{Source: "blast"})
	r2, _ := st.CreateRun(uid, a, NewRun{Source: "blast"})
	keep, _ := st.CreateRun(uid, a, NewRun{Source: "blast"})
	foreign, _ := st.CreateRun(other, foreignAssay, NewRun{Source: "blast"})

	if err := st.SaveArtifact(r1, "xlsx", []byte("book")); err != nil {
		t.Fatal(err)
	}

	// Attempt to delete r1, r2, and someone else's run: only the two owned rows go.
	n, err := st.DeleteResults(uid, []int64{r1, r2, foreign})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("DeleteResults deleted %d, want 2 (owner-scoped)", n)
	}

	if _, err := st.ResultByID(uid, r1); err != ErrNotFound {
		t.Errorf("r1 still present, want ErrNotFound (got %v)", err)
	}
	if _, err := st.ResultByID(uid, keep); err != nil {
		t.Errorf("keep run should survive, got %v", err)
	}
	if _, err := st.ResultByID(other, foreign); err != nil {
		t.Errorf("another user's run must not be deletable, got %v", err)
	}
	// The artifact of the deleted run is gone (FK cascade).
	if _, err := st.Artifact(r1, "xlsx"); err != ErrNotFound {
		t.Errorf("artifact of deleted run = %v, want ErrNotFound", err)
	}

	// Empty id list is a no-op.
	if n, err := st.DeleteResults(uid, nil); err != nil || n != 0 {
		t.Errorf("DeleteResults(nil) = (%d, %v), want (0, nil)", n, err)
	}
}
