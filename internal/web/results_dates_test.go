package web

import (
	"testing"
	"time"
)

// TestDayBoundHelpers checks the YYYY-MM-DD -> RFC3339 UTC conversion used by
// the results date filter: empty/garbage is rejected, and the exclusive upper
// bound is exactly one day after the inclusive lower bound (so the selected day
// is fully included regardless of the server's timezone).
func TestDayBoundHelpers(t *testing.T) {
	if _, ok := dayStartUTC(""); ok {
		t.Error("dayStartUTC(\"\") should not be ok")
	}
	if _, ok := dayEndExclusiveUTC(""); ok {
		t.Error("dayEndExclusiveUTC(\"\") should not be ok")
	}
	if _, ok := dayStartUTC("not-a-date"); ok {
		t.Error("dayStartUTC(garbage) should not be ok")
	}

	startStr, ok := dayStartUTC("2026-08-23")
	if !ok {
		t.Fatal("dayStartUTC(valid) not ok")
	}
	endStr, ok := dayEndExclusiveUTC("2026-08-23")
	if !ok {
		t.Fatal("dayEndExclusiveUTC(valid) not ok")
	}

	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		t.Fatalf("start not RFC3339: %v", err)
	}
	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		t.Fatalf("end not RFC3339: %v", err)
	}
	if d := end.Sub(start); d != 24*time.Hour {
		t.Errorf("end - start = %v, want 24h", d)
	}
	// The lower bound corresponds to local midnight of the requested day.
	if ls := start.In(time.Local); ls.Hour() != 0 || ls.Minute() != 0 {
		t.Errorf("start in local time = %v, want local midnight", ls)
	}
}
