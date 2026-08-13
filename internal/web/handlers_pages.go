package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"AssayManager/internal/analysis"
	"AssayManager/internal/store"
)

type dashboardData struct {
	AssayCount  int
	RunCount    int
	RunningRuns int
	RecentRuns  []dashboardRunRow
}

// catCell is a count with its percentage of the sequence total, plus the CSS
// class chosen for it by the user's dashboard colour thresholds.
type catCell struct {
	Count int
	Pct   float64
	Class string // "mm-good" | "mm-warn" | "mm-bad" | "mm-none"
}

// dashboardRunRow is one recent-completed-run row on the dashboard.
type dashboardRunRow struct {
	ID           int64
	AssayName    string
	AssayVersion string
	When         time.Time
	DateRange    string
	Sequences    int
	Zero         catCell // all categories 0 mismatches
	One          catCell // all categories ≤1 mismatch
	Two          catCell // ≥2 mismatches in any category
	None         catCell // no match in any category
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())

	lineages, err := s.store.ListLineages(user.ID)
	if err != nil {
		s.serverError(w, "list lineages", err)
		return
	}
	results, err := s.store.ListResults(user.ID)
	if err != nil {
		s.serverError(w, "list results", err)
		return
	}
	running := 0
	for _, res := range results {
		if res.Status == "running" {
			running++
		}
	}

	limit := user.DashboardRunCount
	if limit <= 0 {
		limit = 5
	}
	if limit > 50 {
		limit = 50
	}
	recent, err := s.store.RecentDoneResults(user.ID, limit)
	if err != nil {
		s.serverError(w, "recent runs", err)
		return
	}
	rows := make([]dashboardRunRow, 0, len(recent))
	for _, res := range recent {
		row := dashboardRunRow{
			ID:           res.ID,
			AssayName:    res.AssayName,
			AssayVersion: res.AssayVersion,
			When:         res.StartedAt,
			DateRange:    dateRangeLabel(res),
		}
		if parsed, perr := analysis.ParseResult([]byte(res.Report)); perr == nil {
			total := parsed.Summary.TotalSequences
			ov := parsed.Summary.Overall
			row.Sequences = total
			row.Zero = catCell{ov.AllPerfect, dashPct(ov.AllPerfect, total), ""}
			row.One = catCell{ov.MaxOneMismatch, dashPct(ov.MaxOneMismatch, total), ""}
			row.Two = catCell{ov.TwoPlusMismatches, dashPct(ov.TwoPlusMismatches, total), ""}
			row.None = catCell{ov.NoMatch, dashPct(ov.NoMatch, total), "mm-none"}
			// Colour by the user's per-category zones. Only 0-mm is "higher is
			// better"; 1-mm and >1-mm are defect buckets, so higher is worse.
			// "No match" stays neutral (grey).
			row.Zero.Class = mmClass(row.Zero.Pct, user.Mm0Green, user.Mm0Warn, true)
			row.One.Class = mmClass(row.One.Pct, user.Mm1Green, user.Mm1Warn, false)
			row.Two.Class = mmClass(row.Two.Pct, user.Mm2Green, user.Mm2Warn, false)
		}
		rows = append(rows, row)
	}

	pd := s.page(r, "dashboard", "Dashboard")
	pd.Data = dashboardData{
		AssayCount:  len(lineages),
		RunCount:    len(results),
		RunningRuns: running,
		RecentRuns:  rows,
	}
	s.render(w, http.StatusOK, "dashboard", pd)
}

func dashPct(count, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(count) / float64(total) * 100
}

// mmClass picks the dashboard colour class for a percentage given the user's
// green/warn cutoffs. When higherBetter, a larger percentage is better so green
// is the high end (pct ≥ green → good, ≥ warn → warn, else bad); otherwise a
// larger percentage is worse so green is the low end (pct ≤ green → good, ≤ warn
// → warn, else bad).
func mmClass(pct, green, warn float64, higherBetter bool) string {
	if higherBetter {
		switch {
		case pct >= green:
			return "mm-good"
		case pct >= warn:
			return "mm-warn"
		default:
			return "mm-bad"
		}
	}
	switch {
	case pct <= green:
		return "mm-good"
	case pct <= warn:
		return "mm-warn"
	default:
		return "mm-bad"
	}
}

// dateRangeLabel renders a run's reference date range for display: "—" for file
// runs, a formatted range (or "any") for BLAST runs.
func dateRangeLabel(res store.Result) string {
	if res.Source != "blast" {
		return "—"
	}
	switch {
	case res.BlastFrom != "" && res.BlastTo != "":
		return res.BlastFrom + " – " + res.BlastTo
	case res.BlastFrom != "":
		return "from " + res.BlastFrom
	case res.BlastTo != "":
		return "to " + res.BlastTo
	default:
		return "any"
	}
}

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusOK, "profile", s.page(r, "", "Profile"))
}

func (s *Server) handleProfileSave(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())

	cov, errCov := strconv.ParseFloat(strings.TrimSpace(r.FormValue("blast_min_coverage")), 64)
	ident, errID := strconv.ParseFloat(strings.TrimSpace(r.FormValue("blast_min_identity")), 64)
	hits, errHits := strconv.Atoi(strings.TrimSpace(r.FormValue("blast_hitlist_size")))
	runs, errRuns := strconv.Atoi(strings.TrimSpace(r.FormValue("dashboard_run_count")))

	// Dashboard colour thresholds (percentages 0–100). Only 0-mm is higher-is-
	// better (green ≥ warn); the 1-mm and >1-mm defect buckets are higher-is-
	// worse (green ≤ warn).
	mm0g, ok0g := pctField(r, "mm0_green")
	mm0w, ok0w := pctField(r, "mm0_warn")
	mm1g, ok1g := pctField(r, "mm1_green")
	mm1w, ok1w := pctField(r, "mm1_warn")
	mm2g, ok2g := pctField(r, "mm2_green")
	mm2w, ok2w := pctField(r, "mm2_warn")
	mmOK := ok0g && ok0w && ok1g && ok1w && ok2g && ok2w &&
		mm0g >= mm0w && mm1g <= mm1w && mm2g <= mm2w

	if errCov != nil || errID != nil || errHits != nil || errRuns != nil ||
		cov <= 0 || cov > 1 || ident <= 0 || ident > 1 || hits <= 0 ||
		runs < 1 || runs > 50 || !mmOK {
		http.Redirect(w, r, "/profile?msg=bad_profile", http.StatusSeeOther)
		return
	}

	p := store.Profile{
		Name:              strings.TrimSpace(r.FormValue("name")),
		Organisation:      strings.TrimSpace(r.FormValue("organisation")),
		BlastMinCoverage:  cov,
		BlastMinIdentity:  ident,
		BlastHitlistSize:  hits,
		DashboardRunCount: runs,
		Mm0Green:          mm0g,
		Mm0Warn:           mm0w,
		Mm1Green:          mm1g,
		Mm1Warn:           mm1w,
		Mm2Green:          mm2g,
		Mm2Warn:           mm2w,
	}
	if err := s.store.UpdateProfile(user.ID, p); err != nil {
		s.serverError(w, "update profile", err)
		return
	}
	http.Redirect(w, r, "/profile?msg=profile_saved", http.StatusSeeOther)
}

// pctField parses a form field as a percentage in [0, 100]. ok is false if the
// value is missing, unparseable, or out of range.
func pctField(r *http.Request, name string) (v float64, ok bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue(name)), 64)
	if err != nil || v < 0 || v > 100 {
		return 0, false
	}
	return v, true
}
