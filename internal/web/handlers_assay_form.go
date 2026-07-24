package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"AssayManager/internal/assayparser"
	"AssayManager/internal/store"
)

const maxOligoRows = 100

// oligoRow is one editable oligo in the structured ("convenient") editor. Seq
// is the raw seqActual (with /mod/ markers); CleanSeq/Mods/Err are derived for
// the live preview.
type oligoRow struct {
	Name     string
	Function string
	Seq      string
	CleanSeq string
	Mods     []assayparser.Modification
	Err      string
}

// structuredForm is the state of the field-based assay editor. Base carries the
// full assay JSON so fields the form does not expose (off-target taxIDs,
// amplicon source/size, search string) survive a round-trip and a save.
type structuredForm struct {
	IsNew          bool
	Name           string
	Author         string
	Description    string
	Oligos         []oligoRow
	TgtTaxids      string // comma-separated
	RefAmpliconSeq string
	Base           string // hidden full-assay JSON (preserves unexposed fields)
	Error          string
}

// modCatEntry is one display row of the modification reference shown in the
// structured editor. ActsAs holds the base a modification stands in for, or ""
// for a label (fluorophore/quencher/spacer) that contributes no base.
type modCatEntry struct {
	Code    string
	ActsAs  string
	Details string
}

// modList returns assayparser.ModCatalogue as a code-sorted slice for display.
// It is exposed to templates so the reference stays in sync with the parser.
func modList() []modCatEntry {
	out := make([]modCatEntry, 0, len(assayparser.ModCatalogue))
	for _, m := range assayparser.ModCatalogue {
		actsAs := m.ActsAsBase
		if actsAs == assayparser.NonBase {
			actsAs = ""
		}
		out = append(out, modCatEntry{Code: m.Content, ActsAs: actsAs, Details: m.Details})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

func fieldAt(ss []string, i int) string {
	if i < len(ss) {
		return ss[i]
	}
	return ""
}

func rowEmpty(row oligoRow) bool { return row.Name == "" && row.Seq == "" }

func readStructuredForm(r *http.Request) structuredForm {
	f := structuredForm{
		IsNew:          r.FormValue("is_new") == "1",
		Name:           strings.TrimSpace(r.FormValue("name")),
		Author:         strings.TrimSpace(r.FormValue("author")),
		Description:    r.FormValue("description"),
		TgtTaxids:      r.FormValue("tgt_taxids"),
		RefAmpliconSeq: strings.TrimSpace(r.FormValue("ref_amplicon_seq")),
		Base:           r.FormValue("base"),
	}
	names, funcs, seqs := r.Form["oligo_name"], r.Form["oligo_function"], r.Form["oligo_seq"]
	n := len(names)
	if len(funcs) > n {
		n = len(funcs)
	}
	if len(seqs) > n {
		n = len(seqs)
	}
	for i := 0; i < n; i++ {
		f.Oligos = append(f.Oligos, oligoRow{
			Name:     strings.TrimSpace(fieldAt(names, i)),
			Function: fieldAt(funcs, i),
			Seq:      strings.TrimSpace(fieldAt(seqs, i)),
		})
	}
	return f
}

// derive fills CleanSeq/Mods/Err on non-empty rows for the live preview.
func (f *structuredForm) derive() {
	for i := range f.Oligos {
		row := &f.Oligos[i]
		row.CleanSeq, row.Mods, row.Err = "", nil, ""
		if rowEmpty(*row) {
			continue
		}
		o, err := assayparser.MkOligo(row.Name, row.Function, row.Seq)
		if err != nil {
			row.Err = err.Error()
			continue
		}
		row.CleanSeq, row.Mods = o.SeqClean, o.Mods
	}
}

// build assembles a ValidAssay from the form, preserving unexposed fields from
// Base. Version is cleared (assigned by the store on save).
func (f structuredForm) build() (assayparser.ValidAssay, error) {
	var a assayparser.ValidAssay
	if strings.TrimSpace(f.Base) != "" {
		_ = json.Unmarshal([]byte(f.Base), &a) // best-effort; keeps unexposed fields
	}
	a.Header.Name = f.Name
	a.Header.Author = f.Author
	a.Header.Description = f.Description
	a.Header.Version = ""

	oligos := make([]assayparser.Oligo, 0, len(f.Oligos))
	for i, row := range f.Oligos {
		if rowEmpty(row) {
			continue
		}
		o, err := assayparser.MkOligo(row.Name, row.Function, row.Seq)
		if err != nil {
			label := row.Name
			if label == "" {
				label = fmt.Sprintf("row %d", i+1)
			}
			return a, fmt.Errorf("oligo %s: %v", label, err)
		}
		oligos = append(oligos, o)
	}
	a.Oligos.OligoList = oligos

	taxids, err := parseTaxids(f.TgtTaxids)
	if err != nil {
		return a, err
	}
	a.Targets.TgtTaxids = taxids
	a.Targets.RefAmpliconSeq = f.RefAmpliconSeq
	return a, nil
}

func parseTaxids(s string) ([]int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return []int{}, nil
	}
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("invalid taxID %q (use comma-separated numbers, e.g. 562, 11320)", part)
		}
		out = append(out, n)
	}
	return out, nil
}

// structuredFromAssay maps a ValidAssay into field-based editor state.
func structuredFromAssay(a assayparser.ValidAssay, isNew bool) structuredForm {
	f := structuredForm{
		IsNew:          isNew,
		Name:           a.Header.Name,
		Author:         a.Header.Author,
		Description:    a.Header.Description,
		RefAmpliconSeq: a.Targets.RefAmpliconSeq,
	}
	for _, o := range a.Oligos.OligoList {
		f.Oligos = append(f.Oligos, oligoRow{Name: o.Name, Function: o.Function, Seq: o.SeqActual})
	}
	nums := make([]string, len(a.Targets.TgtTaxids))
	for i, t := range a.Targets.TgtTaxids {
		nums[i] = strconv.Itoa(t)
	}
	f.TgtTaxids = strings.Join(nums, ", ")
	if b, err := assayparser.ConvertJson(a); err == nil {
		f.Base = string(b)
	}
	return f
}

func (s *Server) renderStructured(w http.ResponseWriter, r *http.Request, status int, f structuredForm) {
	pd := s.page(r, "assays", "Assay")
	pd.Data = f
	s.render(w, status, "assay_structured", pd)
}

// handleAssayNew (GET /assays/new) shows an empty field-based editor with the
// three typical oligo rows pre-seeded.
func (s *Server) handleAssayNew(w http.ResponseWriter, r *http.Request) {
	s.renderStructured(w, r, http.StatusOK, structuredForm{
		IsNew: true,
		Oligos: []oligoRow{
			{Function: assayparser.FuncForwardPrimer},
			{Function: assayparser.FuncReversePrimer},
			{Function: assayparser.FuncProbe},
		},
	})
}

// handleAssayEdit (GET /assays/{id}/edit) shows the field-based editor prefilled
// from a stored assay.
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
	var a assayparser.ValidAssay
	if err := json.Unmarshal([]byte(assay.Content), &a); err != nil {
		s.serverError(w, "decode assay", err)
		return
	}
	f := structuredFromAssay(a, false)
	f.derive()
	s.renderStructured(w, r, http.StatusOK, f)
}

// handleAssayFormEdit (POST /assays/form/edit) handles the re-render actions:
// add blank rows, remove a row, or refresh the preview.
func (s *Server) handleAssayFormEdit(w http.ResponseWriter, r *http.Request) {
	f := readStructuredForm(r)
	if rm := r.FormValue("remove"); rm != "" {
		if i, err := strconv.Atoi(rm); err == nil && i >= 0 && i < len(f.Oligos) {
			f.Oligos = append(f.Oligos[:i], f.Oligos[i+1:]...)
		}
	} else if r.FormValue("action") == "add" {
		n := 1
		if v, err := strconv.Atoi(strings.TrimSpace(r.FormValue("add_count"))); err == nil && v > 0 {
			n = v
		}
		if len(f.Oligos)+n > maxOligoRows {
			n = maxOligoRows - len(f.Oligos)
		}
		for k := 0; k < n; k++ {
			f.Oligos = append(f.Oligos, oligoRow{})
		}
	}
	f.derive()
	s.renderStructured(w, r, http.StatusOK, f)
}

// handleAssayFormSave (POST /assays/form/save) builds, validates and stores a
// new version from the field-based editor.
func (s *Server) handleAssayFormSave(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	f := readStructuredForm(r)

	renderErr := func(msg string) {
		f.derive()
		f.Error = msg
		s.renderStructured(w, r, http.StatusBadRequest, f)
	}

	a, err := f.build()
	if err != nil {
		renderErr(err.Error())
		return
	}
	if err := validateAssay(a); err != nil {
		renderErr(err.Error())
		return
	}
	saved, err := s.store.SaveNewVersion(user.ID, a, r.FormValue("bump"))
	if err != nil {
		s.serverError(w, "save assay", err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/assays/%d?msg=assay_saved", saved.ID), http.StatusSeeOther)
}

// handleAssayFormToYAML (POST /assays/form/to-yaml) switches to the YAML editor,
// carrying the current data over.
func (s *Server) handleAssayFormToYAML(w http.ResponseWriter, r *http.Request) {
	f := readStructuredForm(r)
	a, err := f.build()
	if err != nil {
		f.derive()
		f.Error = "Fix this before switching to the YAML editor — " + err.Error()
		s.renderStructured(w, r, http.StatusBadRequest, f)
		return
	}
	yamlInput := skeletonYAML
	if !(f.IsNew && a.Header.Name == "" && len(a.Oligos.OligoList) == 0) {
		y, _ := assayparser.ConvertYaml(a)
		yamlInput = string(y)
	}
	pd := s.page(r, "assays", "Assay (YAML)")
	pd.Data = assayFormData{YAMLInput: yamlInput, IsNew: f.IsNew}
	s.render(w, http.StatusOK, "assay_form", pd)
}

// handleAssayYAMLToForm (POST /assays/yaml/to-form) switches from the YAML
// editor back to the field-based editor, carrying the current data over.
func (s *Server) handleAssayYAMLToForm(w http.ResponseWriter, r *http.Request) {
	input := r.FormValue("yaml")
	a, err := buildAssayFromYAML(input)
	if err != nil {
		pd := s.page(r, "assays", "Assay (YAML)")
		pd.Data = assayFormData{YAMLInput: input, Error: "Fix this before switching to the form — " + err.Error()}
		s.render(w, http.StatusBadRequest, "assay_form", pd)
		return
	}
	f := structuredFromAssay(a, false)
	f.derive()
	s.renderStructured(w, r, http.StatusOK, f)
}
