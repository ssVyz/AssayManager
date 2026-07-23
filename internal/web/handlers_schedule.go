package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"AssayManager/internal/store"
)

const scheduleMaxIntervalDays = 3650 // ~10 years

type scheduleFormData struct {
	Schedules      []store.Schedule
	EligibleAssays []batchAssayRow // latest per lineage, BLAST-eligible only
	BlastAvailable bool
	Error          string
}

func (s *Server) buildScheduleFormData(userID int64) (scheduleFormData, error) {
	schedules, err := s.store.ListSchedules(userID)
	if err != nil {
		return scheduleFormData{}, err
	}
	lineages, err := s.store.ListLineages(userID)
	if err != nil {
		return scheduleFormData{}, err
	}
	var eligible []batchAssayRow
	for _, a := range lineages {
		if ok, _ := blastEligibility(a.Content); ok {
			eligible = append(eligible, batchAssayRow{ID: a.ID, Name: a.Name, Version: a.Version, Eligible: true})
		}
	}
	return scheduleFormData{
		Schedules:      schedules,
		EligibleAssays: eligible,
		BlastAvailable: s.analyzer.BlastAvailable(),
	}, nil
}

func (s *Server) handleScheduled(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	data, err := s.buildScheduleFormData(user.ID)
	if err != nil {
		s.serverError(w, "build schedule form", err)
		return
	}
	pd := s.page(r, "scheduled", "Scheduled checks")
	pd.Data = data
	s.render(w, http.StatusOK, "scheduled", pd)
}

func (s *Server) handleScheduleCreate(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())

	renderErr := func(status int, msg string) {
		data, _ := s.buildScheduleFormData(user.ID)
		data.Error = msg
		pd := s.page(r, "scheduled", "Scheduled checks")
		pd.Data = data
		s.render(w, status, "scheduled", pd)
	}

	if !s.analyzer.BlastAvailable() {
		renderErr(http.StatusServiceUnavailable, "BLAST is not configured on this server (no NCBI email).")
		return
	}
	assayID, ok := parseFormID(r, "assay_id")
	if !ok {
		renderErr(http.StatusBadRequest, "Select an assay.")
		return
	}
	assay, err := s.store.AssayByID(user.ID, assayID)
	if errors.Is(err, store.ErrNotFound) {
		renderErr(http.StatusNotFound, "That assay was not found.")
		return
	}
	if err != nil {
		s.serverError(w, "load assay", err)
		return
	}
	if ok, reason := blastEligibility(assay.Content); !ok {
		renderErr(http.StatusBadRequest, "Assay not eligible for BLAST: "+reason)
		return
	}

	lookback, e1 := strconv.Atoi(strings.TrimSpace(r.FormValue("lookback_months")))
	interval, e2 := strconv.Atoi(strings.TrimSpace(r.FormValue("interval_days")))
	if e1 != nil || e2 != nil || lookback < 1 || lookback > blastLookbackMax ||
		interval < 1 || interval > scheduleMaxIntervalDays {
		renderErr(http.StatusBadRequest, "Enter a look-back (1–240 months) and an interval (1–3650 days).")
		return
	}
	next, nerr := time.Parse("2006-01-02", strings.TrimSpace(r.FormValue("next_execution")))
	if nerr != nil {
		renderErr(http.StatusBadRequest, "Enter a valid next-execution date.")
		return
	}

	if _, err := s.store.CreateSchedule(user.ID, assay.ID, "blast", lookback, interval, next); err != nil {
		s.serverError(w, "create schedule", err)
		return
	}
	http.Redirect(w, r, "/scheduled?msg=schedule_created", http.StatusSeeOther)
}

func (s *Server) handleScheduleDelete(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	id, ok := parseFormID(r, "id")
	if !ok {
		http.Redirect(w, r, "/scheduled?msg=not_found", http.StatusSeeOther)
		return
	}
	if err := s.store.DeleteSchedule(user.ID, id); err != nil {
		s.serverError(w, "delete schedule", err)
		return
	}
	http.Redirect(w, r, "/scheduled?msg=schedule_deleted", http.StatusSeeOther)
}
