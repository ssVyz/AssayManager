package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"AssayManager/internal/analysis"
	"AssayManager/internal/assayparser"
	"AssayManager/internal/store"
)

type runFormData struct {
	Assays []store.Assay
}

func (s *Server) handleRunForm(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	assays, err := s.store.ListAllAssays(user.ID)
	if err != nil {
		s.serverError(w, "list assays", err)
		return
	}
	pd := s.page(r, "run", "Run check")
	pd.Data = runFormData{Assays: assays}
	s.render(w, http.StatusOK, "run", pd)
}

func (s *Server) handleRunStart(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	id, ok := pathID(r) // no {id} in path; fall back to form value
	if !ok {
		id, ok = parseFormID(r, "assay_id")
	}
	if !ok {
		http.Redirect(w, r, "/run?msg=not_found", http.StatusSeeOther)
		return
	}

	assay, err := s.store.AssayByID(user.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		http.Redirect(w, r, "/run?msg=not_found", http.StatusSeeOther)
		return
	}
	if err != nil {
		s.serverError(w, "load assay", err)
		return
	}

	params := r.FormValue("params")
	resultID, err := s.store.CreateRun(user.ID, assay, params)
	if err != nil {
		s.serverError(w, "create run", err)
		return
	}

	// Run in the background. Per the MVP model, the results row already exists;
	// this goroutine fills it in when done. If the server dies mid-run, the row
	// is simply left in the "running" state (orphaned).
	go s.runAnalysis(resultID, assay, params)

	http.Redirect(w, r, "/results?msg=run_started", http.StatusSeeOther)
}

func (s *Server) runAnalysis(resultID int64, assay store.Assay, params string) {
	req, err := analysisRequest(assay, params)
	if err != nil {
		_ = s.store.FailRun(resultID, "could not build analysis input: "+err.Error())
		return
	}
	report, err := s.analyzer.Run(context.Background(), req)
	if err != nil {
		_ = s.store.FailRun(resultID, err.Error())
		return
	}
	if err := s.store.CompleteRun(resultID, report.Content); err != nil {
		s.log.Error("complete run", "result", resultID, "err", err)
	}
}

// analysisRequest maps a stored assay to the analyzer's input, grouping oligos
// by their function role and using the clean sequence.
func analysisRequest(assay store.Assay, params string) (analysis.Request, error) {
	var va assayparser.ValidAssay
	if err := json.Unmarshal([]byte(assay.Content), &va); err != nil {
		return analysis.Request{}, err
	}
	req := analysis.Request{Params: map[string]string{}}
	if params != "" {
		req.Params["notes"] = params
	}
	for _, o := range va.Oligos.OligoList {
		ao := analysis.Oligo{ID: o.Name, Seq: o.SeqClean}
		switch o.Function {
		case assayparser.FuncForwardPrimer:
			req.Forward = append(req.Forward, ao)
		case assayparser.FuncReversePrimer:
			req.Reverse = append(req.Reverse, ao)
		case assayparser.FuncProbe:
			req.Probes = append(req.Probes, ao)
		}
	}
	return req, nil
}

func (s *Server) handleResultsList(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	results, err := s.store.ListResults(user.ID)
	if err != nil {
		s.serverError(w, "list results", err)
		return
	}
	pd := s.page(r, "results", "Check results")
	pd.Data = results
	s.render(w, http.StatusOK, "results_list", pd)
}

func (s *Server) handleResultView(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	id, ok := pathID(r)
	if !ok {
		http.Redirect(w, r, "/results?msg=not_found", http.StatusSeeOther)
		return
	}
	res, err := s.store.ResultByID(user.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		http.Redirect(w, r, "/results?msg=not_found", http.StatusSeeOther)
		return
	}
	if err != nil {
		s.serverError(w, "load result", err)
		return
	}
	pd := s.page(r, "results", "Result")
	pd.Data = res
	s.render(w, http.StatusOK, "result_view", pd)
}
