package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"AssayManager/internal/assayparser"
	"AssayManager/internal/store"

	"gopkg.in/yaml.v3"
)

// assayExportFormat is the on-disk version of the export envelope. Bump only if
// the envelope shape changes.
const assayExportFormat = 1

// assayExport is the bulk export/import envelope. The same struct serialises to
// both JSON and YAML (ValidAssay carries both tag sets).
type assayExport struct {
	Format int                      `json:"format" yaml:"format"`
	Assays []assayparser.ValidAssay `json:"assays" yaml:"assays"`
}

// handleAssayExport serialises the selected assays (latest version of each) into
// one downloadable JSON or YAML file.
func (s *Server) handleAssayExport(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())

	ids := r.PostForm["id"]
	if len(ids) == 0 {
		http.Redirect(w, r, "/assays?msg=export_none", http.StatusSeeOther)
		return
	}

	var assays []assayparser.ValidAssay
	for _, raw := range ids {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			continue
		}
		a, err := s.store.AssayByID(user.ID, id)
		if errors.Is(err, store.ErrNotFound) {
			continue // silently drop ids not owned by this user
		}
		if err != nil {
			s.serverError(w, "load assay", err)
			return
		}
		va, err := assayparser.UnwindJson([]byte(a.Content))
		if err != nil {
			s.log.Warn("export: undecodable assay", "id", id, "err", err)
			continue
		}
		assays = append(assays, va)
	}
	if len(assays) == 0 {
		http.Redirect(w, r, "/assays?msg=export_none", http.StatusSeeOther)
		return
	}

	env := assayExport{Format: assayExportFormat, Assays: assays}

	var (
		body []byte
		err  error
		mime string
		ext  string
	)
	switch r.PostFormValue("format") {
	case "yaml":
		body, err = yaml.Marshal(env)
		mime, ext = "application/x-yaml", "yaml"
	default:
		body, err = json.MarshalIndent(env, "", "  ")
		mime, ext = "application/json", "json"
	}
	if err != nil {
		s.serverError(w, "serialize export", err)
		return
	}

	filename := fmt.Sprintf("assaymanager-assays-%s.%s", time.Now().Format("20060102"), ext)
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(body)
}

// handleAssayImport reads an exported JSON/YAML file and inserts its assays,
// preserving each assay's version and skipping any that already exist.
func (s *Server) handleAssayImport(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Redirect(w, r, "/assays?msg=import_nofile", http.StatusSeeOther)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file) // bounded by MaxBytesReader in protectedUpload
	if err != nil {
		http.Redirect(w, r, "/assays?msg=import_bad", http.StatusSeeOther)
		return
	}
	env, err := parseAssayExport(data)
	if err != nil {
		http.Redirect(w, r, "/assays?msg=import_bad", http.StatusSeeOther)
		return
	}

	var imported, skipped, failed int
	for _, va := range env.Assays {
		clean, verr := normalizeImportedAssay(va)
		if verr != nil {
			failed++
			s.log.Warn("import: rejected assay", "name", va.Header.Name, "err", verr)
			continue
		}
		ok, err := s.store.ImportAssay(user.ID, clean)
		if err != nil {
			failed++
			s.log.Warn("import: store rejected assay", "name", clean.Header.Name, "err", err)
			continue
		}
		if ok {
			imported++
		} else {
			skipped++
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/assays?imported=%d&skipped=%d&failed=%d", imported, skipped, failed), http.StatusSeeOther)
}

// parseAssayExport decodes the envelope, sniffing JSON vs YAML by the first
// non-whitespace byte ('{' => JSON, otherwise YAML).
func parseAssayExport(data []byte) (assayExport, error) {
	trimmed := bytes.TrimSpace(data)
	var env assayExport
	if len(trimmed) == 0 {
		return env, errors.New("empty file")
	}
	if trimmed[0] == '{' {
		if err := json.Unmarshal(trimmed, &env); err != nil {
			return env, err
		}
	} else {
		if err := yaml.Unmarshal(trimmed, &env); err != nil {
			return env, err
		}
	}
	if len(env.Assays) == 0 {
		return env, errors.New("no assays in file")
	}
	return env, nil
}

// normalizeImportedAssay re-derives each oligo's clean sequence and modification
// list from its actual sequence (so imported assays are internally consistent,
// even if hand-edited) and validates the header. Unlike the editor path, the
// header version is preserved — imports restore the exact stored version.
func normalizeImportedAssay(a assayparser.ValidAssay) (assayparser.ValidAssay, error) {
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
	if err := validateAssay(a); err != nil {
		return a, err
	}
	if strings.TrimSpace(a.Header.Version) == "" {
		return a, errors.New("missing header.version")
	}
	return a, nil
}

// importSummaryFlash builds the post-import summary message from the redirect
// query parameters, or ("","") if none are present.
func importSummaryFlash(r *http.Request) (text, kind string) {
	q := r.URL.Query()
	if q.Get("imported") == "" && q.Get("skipped") == "" && q.Get("failed") == "" {
		return "", ""
	}
	imported := atoiOr0(q.Get("imported"))
	skipped := atoiOr0(q.Get("skipped"))
	failed := atoiOr0(q.Get("failed"))
	kind = "ok"
	if failed > 0 {
		kind = "err"
	}
	return fmt.Sprintf("Import complete: %d added, %d skipped (already present), %d failed.",
		imported, skipped, failed), kind
}

func atoiOr0(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
