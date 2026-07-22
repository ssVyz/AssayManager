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

// catCell is a count with its percentage of the sequence total.
type catCell struct {
	Count int
	Pct   float64
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
			row.Zero = catCell{ov.AllPerfect, dashPct(ov.AllPerfect, total)}
			row.One = catCell{ov.MaxOneMismatch, dashPct(ov.MaxOneMismatch, total)}
			row.Two = catCell{ov.TwoPlusMismatches, dashPct(ov.TwoPlusMismatches, total)}
			row.None = catCell{ov.NoMatch, dashPct(ov.NoMatch, total)}
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
	if errCov != nil || errID != nil || errHits != nil || errRuns != nil ||
		cov <= 0 || cov > 1 || ident <= 0 || ident > 1 || hits <= 0 ||
		runs < 1 || runs > 50 {
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
	}
	if err := s.store.UpdateProfile(user.ID, p); err != nil {
		s.serverError(w, "update profile", err)
		return
	}
	http.Redirect(w, r, "/profile?msg=profile_saved", http.StatusSeeOther)
}

func (s *Server) handleScheduled(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusOK, "scheduled", s.page(r, "scheduled", "Scheduled checks"))
}
