package store

import (
	"testing"
	"time"

	"AssayManager/internal/assayparser"
)

func seedBlastAssay(t *testing.T, st *Store, ownerID int64, name string) Assay {
	t.Helper()
	fwd, _ := assayparser.MkOligo("F", assayparser.FuncForwardPrimer, "ATGCATGCATGC")
	rev, _ := assayparser.MkOligo("R", assayparser.FuncReversePrimer, "TTCTAGGGTAGG")
	va := assayparser.ValidAssay{
		Header:  assayparser.AssayHeader{Name: name, Author: "t"},
		Oligos:  assayparser.AssayOligos{OligoList: []assayparser.Oligo{fwd, rev}},
		Targets: assayparser.AssayTargets{TgtTaxids: []int{123}, RefAmpliconSeq: "AATACTAATCGT"},
	}
	a, err := st.SaveNewVersion(ownerID, va, "minor")
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestScheduleLifecycle(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	uid, err := st.CreateUser("u", "h")
	if err != nil {
		t.Fatal(err)
	}
	a := seedBlastAssay(t, st, uid, "SchedAssay")

	pastID, err := st.CreateSchedule(uid, a.ID, "blast", 12, 30, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateSchedule(uid, a.ID, "blast", 6, 7, time.Now().Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}

	list, err := st.ListSchedules(uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("ListSchedules = %d, want 2", len(list))
	}
	if list[0].NextExecution.After(list[1].NextExecution) {
		t.Errorf("schedules not sorted ascending by next_execution")
	}
	if list[0].AssayName != "SchedAssay" {
		t.Errorf("AssayName = %q, want SchedAssay", list[0].AssayName)
	}

	due, err := st.DueSchedules(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].ID != pastID {
		t.Fatalf("DueSchedules = %+v, want only the past one (%d)", due, pastID)
	}

	if err := st.AdvanceSchedule(pastID, time.Now().Add(72*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if due, _ := st.DueSchedules(time.Now()); len(due) != 0 {
		t.Errorf("after advance, DueSchedules = %d, want 0", len(due))
	}

	// A result linked to the schedule must survive schedule deletion (SET NULL).
	if _, err := st.CreateRun(uid, a, NewRun{Source: "blast", ScheduleID: pastID}); err != nil {
		t.Fatal(err)
	}
	before, _ := st.ListResults(uid)
	if err := st.DeleteSchedule(uid, pastID); err != nil {
		t.Fatal(err)
	}
	after, _ := st.ListResults(uid)
	if len(after) != len(before) {
		t.Errorf("deleting a schedule changed result count %d -> %d (should preserve history)", len(before), len(after))
	}
	if list, _ := st.ListSchedules(uid); len(list) != 1 {
		t.Errorf("after delete, ListSchedules = %d, want 1", len(list))
	}
}
