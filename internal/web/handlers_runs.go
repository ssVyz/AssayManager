package web

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"AssayManager/internal/analysis"
	"AssayManager/internal/assayparser"
	"AssayManager/internal/store"
)

type runFormData struct {
	Assays    []store.Assay
	Available bool
	Error     string
}

func (s *Server) handleRunForm(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	assays, err := s.store.ListAllAssays(user.ID)
	if err != nil {
		s.serverError(w, "list assays", err)
		return
	}
	pd := s.page(r, "run", "Run check")
	pd.Data = runFormData{Assays: assays, Available: s.analyzer.Available()}
	s.render(w, http.StatusOK, "run", pd)
}

func (s *Server) handleRunStart(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())

	renderErr := func(status int, msg string) {
		assays, _ := s.store.ListAllAssays(user.ID)
		pd := s.page(r, "run", "Run check")
		pd.Data = runFormData{Assays: assays, Available: s.analyzer.Available(), Error: msg}
		s.render(w, status, "run", pd)
	}

	if !s.analyzer.Available() {
		renderErr(http.StatusServiceUnavailable, "The analysis tool is not available on this server.")
		return
	}

	id, ok := parseFormID(r, "assay_id")
	if !ok {
		renderErr(http.StatusBadRequest, "Select an assay to run.")
		return
	}
	assay, err := s.store.AssayByID(user.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		renderErr(http.StatusNotFound, "That assay was not found.")
		return
	}
	if err != nil {
		s.serverError(w, "load assay", err)
		return
	}

	// Analysis-eligibility gate (re-validate the assay for the tool's needs).
	if err := validateAssayForAnalysis(assay.Content); err != nil {
		renderErr(http.StatusBadRequest, "Assay not eligible for analysis: "+err.Error())
		return
	}

	// Reference FASTA upload → temp file (persisted past this request).
	file, header, err := r.FormFile("reference")
	if err != nil {
		renderErr(http.StatusBadRequest, "Upload a reference sequence set (FASTA).")
		return
	}
	defer file.Close()

	refPath, err := saveReferenceUpload(file)
	if err != nil {
		if errors.Is(err, errEmptyOrNotFasta) {
			renderErr(http.StatusBadRequest, "The uploaded file is empty or does not look like FASTA (expected a line starting with '>').")
			return
		}
		s.serverError(w, "save reference upload", err)
		return
	}

	referenceName := "reference.fasta"
	if header != nil && header.Filename != "" {
		referenceName = header.Filename
	}
	params := strings.TrimSpace(r.FormValue("params"))

	resultID, err := s.store.CreateRun(user.ID, assay, params, referenceName)
	if err != nil {
		os.Remove(refPath)
		s.serverError(w, "create run", err)
		return
	}

	// Run in the background (bounded). The results row already exists; this
	// goroutine fills it in when done. If the server dies mid-run, the row is
	// left "running" (orphaned), per the MVP model.
	go s.runAnalysis(resultID, assay, refPath)

	http.Redirect(w, r, "/results?msg=run_started", http.StatusSeeOther)
}

func (s *Server) runAnalysis(resultID int64, assay store.Assay, referencePath string) {
	defer os.Remove(referencePath)

	// Bound concurrency: each run is internally parallel, so cap simultaneous runs.
	s.runSem <- struct{}{}
	defer func() { <-s.runSem }()

	rep, err := s.analyzer.Run(context.Background(), analysis.Request{
		AssayJSON:     []byte(assay.Content),
		ReferencePath: referencePath,
	})
	if err != nil {
		if ferr := s.store.FailRun(resultID, err.Error()); ferr != nil {
			s.log.Error("fail run", "result", resultID, "err", ferr)
		}
		return
	}
	if err := s.store.CompleteRun(resultID, string(rep.RawJSON), rep.ToolName, rep.ToolVersion, rep.SchemaVersion); err != nil {
		s.log.Error("complete run", "result", resultID, "err", err)
		return
	}
	for _, a := range rep.Artifacts {
		if err := s.store.SaveArtifact(resultID, a.Kind, a.Content); err != nil {
			s.log.Error("save artifact", "result", resultID, "kind", a.Kind, "err", err)
		}
	}
}

var errEmptyOrNotFasta = errors.New("empty or not FASTA")

// saveReferenceUpload streams an uploaded reference to a temp file, doing a
// light FASTA sanity check on the first non-whitespace byte. Returns the path;
// the caller owns cleanup.
func saveReferenceUpload(file io.Reader) (string, error) {
	br := bufio.NewReader(file)
	// Peek does not consume, so the subsequent Copy still sees the whole stream.
	prefix, _ := br.Peek(512)
	if !startsWithFastaHeader(prefix) {
		return "", errEmptyOrNotFasta
	}

	tmp, err := os.CreateTemp("", "am-ref-*.fasta")
	if err != nil {
		return "", err
	}
	defer tmp.Close()
	if _, err := io.Copy(tmp, br); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

// startsWithFastaHeader reports whether the first non-whitespace byte is '>'.
func startsWithFastaHeader(b []byte) bool {
	for _, c := range b {
		switch c {
		case ' ', '\t', '\r', '\n', '\v', '\f':
			continue
		case '>':
			return true
		default:
			return false
		}
	}
	return false
}

// validateAssayForAnalysis re-checks the stored assay against the analysis
// pipeline's requirements: at least one forward and one reverse primer (by
// function role, with a non-empty clean sequence), and unique oligo names.
func validateAssayForAnalysis(content string) error {
	var va assayparser.ValidAssay
	if err := json.Unmarshal([]byte(content), &va); err != nil {
		return errors.New("stored assay could not be decoded")
	}
	seen := map[string]bool{}
	fwd, rev := 0, 0
	for _, o := range va.Oligos.OligoList {
		if o.Name != "" {
			if seen[o.Name] {
				return fmt.Errorf("duplicate oligo name %q (names must be unique)", o.Name)
			}
			seen[o.Name] = true
		}
		if strings.TrimSpace(o.SeqClean) == "" {
			continue
		}
		switch o.Function {
		case assayparser.FuncForwardPrimer:
			fwd++
		case assayparser.FuncReversePrimer:
			rev++
		}
	}
	if fwd == 0 {
		return errors.New("needs at least one forward-primer oligo with a sequence")
	}
	if rev == 0 {
		return errors.New("needs at least one reverse-primer oligo with a sequence")
	}
	return nil
}

type resultViewData struct {
	Result    store.Result
	View      *analysis.ResultView // nil if the report is not structured JSON
	Downloads []downloadLink
}

type downloadLink struct {
	Kind  string
	Label string
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

	vd := resultViewData{Result: res}
	if res.Report != "" {
		if parsed, perr := analysis.ParseResult([]byte(res.Report)); perr == nil {
			v := parsed.Table()
			vd.View = &v
		}
	}
	vd.Downloads = s.downloadsFor(res)

	pd := s.page(r, "results", "Result")
	pd.Data = vd
	s.render(w, http.StatusOK, "result_view", pd)
}

var downloadLabels = map[string]string{
	"xlsx": "Excel (.xlsx)",
	"txt":  "Text (.txt)",
	"json": "JSON",
}

// downloadsFor lists the downloadable outputs for a result: stored artifacts
// plus JSON (served from the stored report), in a stable order.
func (s *Server) downloadsFor(res store.Result) []downloadLink {
	kinds, err := s.store.ArtifactKinds(res.ID)
	if err != nil {
		s.log.Error("artifact kinds", "result", res.ID, "err", err)
	}
	has := map[string]bool{}
	for _, k := range kinds {
		has[k] = true
	}
	var out []downloadLink
	for _, k := range []string{"xlsx", "txt"} {
		if has[k] {
			out = append(out, downloadLink{Kind: k, Label: downloadLabels[k]})
		}
	}
	if res.Report != "" {
		out = append(out, downloadLink{Kind: "json", Label: downloadLabels["json"]})
	}
	return out
}

func (s *Server) handleResultDownload(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	res, err := s.store.ResultByID(user.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.serverError(w, "load result", err)
		return
	}

	kind := r.PathValue("kind")
	var content []byte
	var mime string
	switch kind {
	case "json":
		if res.Report == "" {
			http.NotFound(w, r)
			return
		}
		content = []byte(res.Report)
		mime = "application/json"
	case "xlsx":
		content, err = s.store.Artifact(id, "xlsx")
		mime = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case "txt":
		content, err = s.store.Artifact(id, "txt")
		mime = "text/plain; charset=utf-8"
	default:
		http.Error(w, "unknown download type", http.StatusBadRequest)
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.serverError(w, "load artifact", err)
		return
	}

	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", downloadName(res, kind)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(content)
}

// downloadName builds a safe download filename from the assay identity.
func downloadName(res store.Result, ext string) string {
	base := sanitizeFilename(res.AssayName + "_" + res.AssayVersion)
	if base == "" || base == "_" {
		base = fmt.Sprintf("result_%d", res.ID)
	}
	return base + "." + ext
}

func sanitizeFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}
