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
	"strconv"
	"strings"

	"AssayManager/internal/analysis"
	"AssayManager/internal/assayparser"
	"AssayManager/internal/store"
)

type runFormData struct {
	Assays         []store.Assay   // all versions, for the single-run selector
	BatchAssays    []batchAssayRow // latest per lineage, for the batch-BLAST list
	Available      bool
	BlastAvailable bool
	Error          string
}

// batchAssayRow is one row of the batch-BLAST list: the latest version of a
// lineage, with whether it's eligible for a BLAST check and, if not, why.
type batchAssayRow struct {
	ID       int64
	Name     string
	Version  string
	Eligible bool
	Reason   string
}

// buildRunFormData assembles the data for the Run page: the single-run assay
// selector, and the batch-BLAST list (latest version of each lineage annotated
// with BLAST eligibility).
func (s *Server) buildRunFormData(userID int64) (runFormData, error) {
	assays, err := s.store.ListAllAssays(userID)
	if err != nil {
		return runFormData{}, err
	}
	lineages, err := s.store.ListLineages(userID)
	if err != nil {
		return runFormData{}, err
	}
	batch := make([]batchAssayRow, 0, len(lineages))
	for _, a := range lineages {
		ok, reason := blastEligibility(a.Content)
		batch = append(batch, batchAssayRow{ID: a.ID, Name: a.Name, Version: a.Version, Eligible: ok, Reason: reason})
	}
	return runFormData{
		Assays:         assays,
		BatchAssays:    batch,
		Available:      s.analyzer.Available(),
		BlastAvailable: s.analyzer.BlastAvailable(),
	}, nil
}

func (s *Server) handleRunForm(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	data, err := s.buildRunFormData(user.ID)
	if err != nil {
		s.serverError(w, "build run form", err)
		return
	}
	pd := s.page(r, "run", "Run check")
	pd.Data = data
	s.render(w, http.StatusOK, "run", pd)
}

func (s *Server) handleRunStart(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())

	renderErr := func(status int, msg string) {
		data, _ := s.buildRunFormData(user.ID)
		data.Error = msg
		pd := s.page(r, "run", "Run check")
		pd.Data = data
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

	params := strings.TrimSpace(r.FormValue("params"))
	req := analysis.Request{AssayJSON: []byte(assay.Content)}
	var referenceName, cleanupPath string

	switch r.FormValue("source") {
	case "blast":
		if !s.analyzer.BlastAvailable() {
			renderErr(http.StatusServiceUnavailable, "BLAST is not configured on this server (no NCBI email).")
			return
		}
		query, taxids, verr := blastInputsFromAssay(assay.Content)
		if verr != nil {
			renderErr(http.StatusBadRequest, verr.Error())
			return
		}
		from := slashDate(r.FormValue("blast_from"))
		to := slashDate(r.FormValue("blast_to"))
		req.Blast = &analysis.BlastParams{
			Query:       query,
			TaxIDs:      taxids,
			From:        from,
			To:          to,
			MinCoverage: user.BlastMinCoverage,
			MinIdentity: user.BlastMinIdentity,
			HitlistSize: user.BlastHitlistSize,
		}
		referenceName = blastDescriptor(taxids, from, to)

	default: // file upload
		file, header, ferr := r.FormFile("reference")
		if ferr != nil {
			renderErr(http.StatusBadRequest, "Upload a reference sequence set (FASTA).")
			return
		}
		defer file.Close()
		refPath, serr := saveReferenceUpload(file)
		if serr != nil {
			if errors.Is(serr, errEmptyOrNotFasta) {
				renderErr(http.StatusBadRequest, "The uploaded file is empty or does not look like FASTA (expected a line starting with '>').")
				return
			}
			s.serverError(w, "save reference upload", serr)
			return
		}
		req.ReferencePath = refPath
		cleanupPath = refPath
		referenceName = "reference.fasta"
		if header != nil && header.Filename != "" {
			referenceName = header.Filename
		}
	}

	resultID, err := s.store.CreateRun(user.ID, assay, params, referenceName)
	if err != nil {
		if cleanupPath != "" {
			os.Remove(cleanupPath)
		}
		s.serverError(w, "create run", err)
		return
	}

	// Run in the background (bounded). The results row already exists; this
	// goroutine fills it in when done. If the server dies mid-run, the row is
	// left "running" (orphaned), per the MVP model.
	go s.runAnalysis(resultID, req, cleanupPath)

	http.Redirect(w, r, "/results?msg=run_started", http.StatusSeeOther)
}

// blastInputsFromAssay pulls the BLAST query region (refAmpliconSeq) and target
// taxIDs (tgtTaxids) from a stored assay, validating that both are present.
func blastInputsFromAssay(content string) (query string, taxids []int, err error) {
	var va assayparser.ValidAssay
	if e := json.Unmarshal([]byte(content), &va); e != nil {
		return "", nil, errors.New("stored assay could not be decoded")
	}
	query = strings.TrimSpace(va.Targets.RefAmpliconSeq)
	if query == "" {
		return "", nil, errors.New("this assay has no reference amplicon (targets.refAmpliconSeq), which BLAST requires")
	}
	if len(va.Targets.TgtTaxids) == 0 {
		return "", nil, errors.New("this assay has no target taxIDs (targets.tgtTaxids), which BLAST requires")
	}
	return query, va.Targets.TgtTaxids, nil
}

// slashDate converts an HTML date input (YYYY-MM-DD) to the tool's YYYY/MM/DD.
func slashDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return strings.ReplaceAll(s, "-", "/")
}

func blastDescriptor(taxids []int, from, to string) string {
	d := "NCBI BLAST · taxids " + joinInts(taxids)
	switch {
	case from != "" && to != "":
		d += " · " + from + "–" + to
	case from != "":
		d += " · from " + from
	case to != "":
		d += " · to " + to
	}
	return d
}

func joinInts(ns []int) string {
	parts := make([]string, len(ns))
	for i, n := range ns {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ", ")
}

// blastEligibility reports whether an assay can run a BLAST check, and if not,
// a short reason for display.
func blastEligibility(content string) (bool, string) {
	if err := validateAssayForAnalysis(content); err != nil {
		return false, err.Error()
	}
	var va assayparser.ValidAssay
	if err := json.Unmarshal([]byte(content), &va); err != nil {
		return false, "unreadable assay"
	}
	if strings.TrimSpace(va.Targets.RefAmpliconSeq) == "" {
		return false, "no reference amplicon"
	}
	if len(va.Targets.TgtTaxids) == 0 {
		return false, "no target taxIDs"
	}
	return true, ""
}

// handleRunBatch starts a BLAST check for each selected assay (latest version),
// with one shared publication-date range. Ineligible or vanished selections are
// skipped and counted. Each run is a bounded background goroutine, as for a
// single run.
func (s *Server) handleRunBatch(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())

	if !s.analyzer.BlastAvailable() {
		http.Redirect(w, r, "/run?msg=blast_off", http.StatusSeeOther)
		return
	}
	ids := r.PostForm["id"]
	if len(ids) == 0 {
		http.Redirect(w, r, "/run?msg=batch_none", http.StatusSeeOther)
		return
	}
	from := slashDate(r.FormValue("blast_from"))
	to := slashDate(r.FormValue("blast_to"))

	var started, skipped int
	for _, raw := range ids {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			skipped++
			continue
		}
		assay, err := s.store.AssayByID(user.ID, id)
		if errors.Is(err, store.ErrNotFound) {
			skipped++
			continue
		}
		if err != nil {
			s.serverError(w, "load assay", err)
			return
		}
		query, taxids, verr := blastInputsFromAssay(assay.Content)
		if verr != nil || validateAssayForAnalysis(assay.Content) != nil {
			skipped++
			continue
		}

		req := analysis.Request{
			AssayJSON: []byte(assay.Content),
			Blast: &analysis.BlastParams{
				Query:       query,
				TaxIDs:      taxids,
				From:        from,
				To:          to,
				MinCoverage: user.BlastMinCoverage,
				MinIdentity: user.BlastMinIdentity,
				HitlistSize: user.BlastHitlistSize,
			},
		}
		resultID, err := s.store.CreateRun(user.ID, assay, "", blastDescriptor(taxids, from, to))
		if err != nil {
			s.serverError(w, "create run", err)
			return
		}
		go s.runAnalysis(resultID, req, "")
		started++
	}

	http.Redirect(w, r, fmt.Sprintf("/results?started=%d&skipped=%d", started, skipped), http.StatusSeeOther)
}

// runBatchSummaryFlash builds the post-batch summary from redirect query params,
// or ("","") if none present.
func runBatchSummaryFlash(r *http.Request) (text, kind string) {
	q := r.URL.Query()
	if q.Get("started") == "" && q.Get("skipped") == "" {
		return "", ""
	}
	started := atoiOr0(q.Get("started"))
	skipped := atoiOr0(q.Get("skipped"))
	if started == 0 {
		return fmt.Sprintf("No BLAST runs started (%d skipped as ineligible).", skipped), "err"
	}
	if skipped > 0 {
		return fmt.Sprintf("Started %d BLAST run(s); skipped %d ineligible.", started, skipped), "ok"
	}
	return fmt.Sprintf("Started %d BLAST run(s).", started), "ok"
}

func (s *Server) runAnalysis(resultID int64, req analysis.Request, cleanupPath string) {
	if cleanupPath != "" {
		defer os.Remove(cleanupPath)
	}

	// Bound concurrency: each run is internally parallel, so cap simultaneous runs.
	s.runSem <- struct{}{}
	defer func() { <-s.runSem }()

	rep, err := s.analyzer.Run(context.Background(), req)
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
	if txt, kind := runBatchSummaryFlash(r); txt != "" {
		pd.Flash, pd.FlashKind = txt, kind
	}
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
