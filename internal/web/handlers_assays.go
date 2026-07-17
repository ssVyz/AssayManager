package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"AssayManager/internal/assayparser"
	"AssayManager/internal/store"

	"gopkg.in/yaml.v3"
)

const skeletonYAML = `header:
  name: "New assay"
  author: ""
oligos:
  oligoList:
    - name: "Fwd"
      function: "forward-primer"
      seqActual: "ATGCATGCATGCATGCAT"
    - name: "Rev"
      function: "reverse-primer"
      seqActual: "TTTTGGGGCCCCAAAATT"
    - name: "Probe"
      function: "probe"
      seqActual: "/56-FAM/ATGCATGCATGCAT/3BHQ_1/"
targets:
  tgtTaxids: []
  offTaxids: []
`

type assayFormData struct {
	YAMLInput string
	IsNew     bool
	Error     string
	Preview   *assayPreview
}

type assayPreview struct {
	Parsed assayparser.ValidAssay
	YAML   string
	JSON   string
}

type assayViewData struct {
	Assay  store.Assay
	Parsed assayparser.ValidAssay
	YAML   string
	JSON   string
}

type assayHistoryData struct {
	Name     string
	Versions []assayRow
}

// assayRow is a list-friendly projection of an assay with the author pulled out
// of the JSON content.
type assayRow struct {
	ID        int64
	Name      string
	Version   string
	Author    string
	CreatedAt time.Time
}

func toAssayRow(a store.Assay) assayRow {
	row := assayRow{ID: a.ID, Name: a.Name, Version: a.Version, CreatedAt: a.CreatedAt}
	var va assayparser.ValidAssay
	if json.Unmarshal([]byte(a.Content), &va) == nil {
		row.Author = va.Header.Author
	}
	return row
}

func toAssayRows(list []store.Assay) []assayRow {
	rows := make([]assayRow, 0, len(list))
	for _, a := range list {
		rows = append(rows, toAssayRow(a))
	}
	return rows
}

func (s *Server) handleAssaysList(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	lineages, err := s.store.ListLineages(user.ID)
	if err != nil {
		s.serverError(w, "list lineages", err)
		return
	}
	pd := s.page(r, "assays", "Assays")
	pd.Data = toAssayRows(lineages)
	s.render(w, http.StatusOK, "assays_list", pd)
}

func (s *Server) handleAssayNew(w http.ResponseWriter, r *http.Request) {
	pd := s.page(r, "assays", "New assay")
	pd.Data = assayFormData{YAMLInput: skeletonYAML, IsNew: true}
	s.render(w, http.StatusOK, "assay_form", pd)
}

func (s *Server) handleAssayEdit(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	id, ok := pathID(r)
	if !ok {
		http.Redirect(w, r, "/assays?msg=not_found", http.StatusSeeOther)
		return
	}
	assay, err := s.store.AssayByID(user.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		http.Redirect(w, r, "/assays?msg=not_found", http.StatusSeeOther)
		return
	}
	if err != nil {
		s.serverError(w, "load assay", err)
		return
	}

	var parsed assayparser.ValidAssay
	if err := json.Unmarshal([]byte(assay.Content), &parsed); err != nil {
		s.serverError(w, "decode assay", err)
		return
	}
	y, _ := assayparser.ConvertYaml(parsed)

	pd := s.page(r, "assays", "Edit assay")
	pd.Data = assayFormData{YAMLInput: string(y), IsNew: false}
	s.render(w, http.StatusOK, "assay_form", pd)
}

func (s *Server) handleAssayPreview(w http.ResponseWriter, r *http.Request) {
	input := r.FormValue("yaml")
	form := assayFormData{YAMLInput: input}

	a, err := buildAssayFromYAML(input)
	if err != nil {
		form.Error = err.Error()
	} else if verr := validateAssay(a); verr != nil {
		form.Error = verr.Error()
		if p, ok := makePreview(a); ok {
			form.Preview = p
		}
	} else if p, ok := makePreview(a); ok {
		form.Preview = p
	}

	pd := s.page(r, "assays", "Assay preview")
	pd.Data = form
	s.render(w, http.StatusOK, "assay_form", pd)
}

func (s *Server) handleAssaySave(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	input := r.FormValue("yaml")
	bump := r.FormValue("bump")

	renderErr := func(msg string) {
		pd := s.page(r, "assays", "Assay")
		pd.Data = assayFormData{YAMLInput: input, Error: msg}
		s.render(w, http.StatusBadRequest, "assay_form", pd)
	}

	a, err := buildAssayFromYAML(input)
	if err != nil {
		renderErr(err.Error())
		return
	}
	if err := validateAssay(a); err != nil {
		renderErr(err.Error())
		return
	}

	saved, err := s.store.SaveNewVersion(user.ID, a, bump)
	if err != nil {
		s.serverError(w, "save assay", err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/assays/%d?msg=assay_saved", saved.ID), http.StatusSeeOther)
}

func (s *Server) handleAssayView(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	id, ok := pathID(r)
	if !ok {
		http.Redirect(w, r, "/assays?msg=not_found", http.StatusSeeOther)
		return
	}
	assay, err := s.store.AssayByID(user.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		http.Redirect(w, r, "/assays?msg=not_found", http.StatusSeeOther)
		return
	}
	if err != nil {
		s.serverError(w, "load assay", err)
		return
	}

	var parsed assayparser.ValidAssay
	if err := json.Unmarshal([]byte(assay.Content), &parsed); err != nil {
		s.serverError(w, "decode assay", err)
		return
	}
	y, _ := assayparser.ConvertYaml(parsed)
	pretty, _ := json.MarshalIndent(parsed, "", "  ")

	pd := s.page(r, "assays", assay.Name+" "+assay.Version)
	pd.Data = assayViewData{Assay: assay, Parsed: parsed, YAML: string(y), JSON: string(pretty)}
	s.render(w, http.StatusOK, "assay_view", pd)
}

func (s *Server) handleAssayHistory(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	name := r.URL.Query().Get("name")
	versions, err := s.store.ListVersions(user.ID, name)
	if err != nil {
		s.serverError(w, "list versions", err)
		return
	}
	pd := s.page(r, "assays", "History: "+name)
	pd.Data = assayHistoryData{Name: name, Versions: toAssayRows(versions)}
	s.render(w, http.StatusOK, "assay_history", pd)
}

func (s *Server) handleAssayDelete(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	name := r.FormValue("name")
	if _, err := s.store.DeleteLineage(user.ID, name); err != nil {
		s.serverError(w, "delete assay", err)
		return
	}
	http.Redirect(w, r, "/assays?msg=assay_deleted", http.StatusSeeOther)
}

// buildAssayFromYAML parses user YAML into a ValidAssay and re-derives each
// oligo's clean sequence and modification list from its seqActual (the sequence
// with inline /mod/ markers is the source of truth). Version is cleared; it is
// assigned by the store on save.
func buildAssayFromYAML(input string) (assayparser.ValidAssay, error) {
	var a assayparser.ValidAssay
	if err := yaml.Unmarshal([]byte(input), &a); err != nil {
		return a, fmt.Errorf("could not parse YAML: %v", err)
	}
	for i := range a.Oligos.OligoList {
		o := a.Oligos.OligoList[i]
		built, err := assayparser.MkOligo(o.Name, o.Function, o.SeqActual)
		if err != nil {
			label := o.Name
			if label == "" {
				label = fmt.Sprintf("#%d", i+1)
			}
			return a, fmt.Errorf("oligo %s: %v", label, err)
		}
		a.Oligos.OligoList[i] = built
	}
	a.Header.Version = ""
	return a, nil
}

func validateAssay(a assayparser.ValidAssay) error {
	if strings.TrimSpace(a.Header.Name) == "" {
		return errors.New("header.name is required")
	}
	if strings.TrimSpace(a.Header.Author) == "" {
		return errors.New("header.author is required")
	}
	return nil
}

func makePreview(a assayparser.ValidAssay) (*assayPreview, bool) {
	y, err := assayparser.ConvertYaml(a)
	if err != nil {
		return nil, false
	}
	pretty, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return nil, false
	}
	return &assayPreview{Parsed: a, YAML: string(y), JSON: string(pretty)}, true
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil
}

func parseFormID(r *http.Request, field string) (int64, bool) {
	id, err := strconv.ParseInt(r.FormValue(field), 10, 64)
	return id, err == nil
}
