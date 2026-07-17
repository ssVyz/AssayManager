package web

import (
	"net/http"
	"strings"
)

type dashboardData struct {
	AssayCount  int
	RunCount    int
	RunningRuns int
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

	pd := s.page(r, "dashboard", "Dashboard")
	pd.Data = dashboardData{
		AssayCount:  len(lineages),
		RunCount:    len(results),
		RunningRuns: running,
	}
	s.render(w, http.StatusOK, "dashboard", pd)
}

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusOK, "profile", s.page(r, "", "Profile"))
}

func (s *Server) handleProfileSave(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	name := strings.TrimSpace(r.FormValue("name"))
	org := strings.TrimSpace(r.FormValue("organisation"))
	if err := s.store.UpdateProfile(user.ID, name, org); err != nil {
		s.serverError(w, "update profile", err)
		return
	}
	http.Redirect(w, r, "/profile?msg=profile_saved", http.StatusSeeOther)
}

func (s *Server) handleScheduled(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusOK, "scheduled", s.page(r, "scheduled", "Scheduled checks"))
}
