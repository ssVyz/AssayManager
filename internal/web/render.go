package web

import (
	"bytes"
	"embed"
	"html/template"
	"net/http"
	"time"
)

//go:embed templates/*.html templates/partials/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// pages are the content templates rendered inside the normal app layout; each is
// parsed together with the layout and the shared partials.
var pages = []string{
	"login", "register", "dashboard", "profile",
	"assays_list", "assay_form", "assay_structured", "assay_view", "assay_history",
	"run", "scheduled", "results_list", "results_delete_confirm", "result_view",
}

// printPages render inside a standalone print layout (no app chrome) instead of
// the normal layout — they are meant to be exported to PDF via the browser.
var printPages = []string{"results_report"}

// partials are shared template files (each only defines named sub-templates) that
// are parsed into every page so any page can invoke them.
var partials = []string{"templates/partials/result_detail.html"}

func parseTemplates() (map[string]*template.Template, error) {
	funcs := template.FuncMap{"fmtTime": fmtTime, "modList": modList}
	m := make(map[string]*template.Template, len(pages)+len(printPages))
	parse := func(root, page string) error {
		files := append([]string{"templates/" + root}, partials...)
		files = append(files, "templates/"+page+".html")
		t, err := template.New(root).Funcs(funcs).ParseFS(templateFS, files...)
		if err != nil {
			return err
		}
		m[page] = t
		return nil
	}
	for _, p := range pages {
		if err := parse("layout.html", p); err != nil {
			return nil, err
		}
	}
	for _, p := range printPages {
		if err := parse("print_layout.html", p); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// rootLayout maps each page to the name of its root (layout) template, so render
// executes the correct outer document.
var rootLayout = func() map[string]string {
	m := make(map[string]string)
	for _, p := range pages {
		m[p] = "layout.html"
	}
	for _, p := range printPages {
		m[p] = "print_layout.html"
	}
	return m
}()

// pageData is the template context shared by every page.
type pageData struct {
	Title     string
	ActiveNav string
	User      any // *store.User
	CSRF      string
	Flash     string
	FlashKind string // "ok" | "err"
	Data      any
}

// page builds the common page context from the request.
func (s *Server) page(r *http.Request, active, title string) pageData {
	pd := pageData{ActiveNav: active, Title: title}
	if u := userFrom(r.Context()); u != nil {
		pd.User = u
	}
	if sess := sessionFrom(r.Context()); sess != nil {
		pd.CSRF = sess.CSRFToken
	}
	pd.Flash, pd.FlashKind = flashFromQuery(r)
	return pd
}

func (s *Server) render(w http.ResponseWriter, status int, page string, pd pageData) {
	t, ok := s.tmpl[page]
	if !ok {
		s.log.Error("unknown template", "page", page)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	root := rootLayout[page]
	if root == "" {
		root = "layout.html"
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, root, pd); err != nil {
		s.log.Error("render failed", "page", page, "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

var flashes = map[string]struct{ Kind, Text string }{
	"registered":       {"ok", "Account created — please log in."},
	"loggedout":        {"ok", "You have been logged out."},
	"profile_saved":    {"ok", "Profile updated."},
	"assay_saved":      {"ok", "Assay version saved."},
	"assay_deleted":    {"ok", "Assay deleted."},
	"run_started":      {"ok", "Analysis run started — see Check results."},
	"badlogin":         {"err", "Invalid username or password."},
	"login_required":   {"err", "Please log in to continue."},
	"pw_mismatch":      {"err", "Passwords do not match."},
	"pw_empty":         {"err", "Password must not be empty."},
	"user_taken":       {"err", "That username is already taken."},
	"bad_register":     {"err", "Provide a username and matching, non-empty passwords."},
	"bad_profile":      {"err", "Check your settings: BLAST coverage and identity in 0–1, hitlist a positive integer, recent-runs a whole number 1–50, and colour thresholds in 0–100 with green on the correct side of yellow for each category."},
	"not_found":        {"err", "That item was not found."},
	"export_none":      {"err", "Select at least one assay to export."},
	"results_deleted":  {"ok", "Selected results deleted."},
	"delete_none":      {"err", "Select at least one result to delete."},
	"report_none":      {"err", "Select at least one result to export."},
	"import_nofile":    {"err", "Choose a file to import."},
	"import_bad":       {"err", "Could not read that file as an assay export (expected JSON or YAML with an 'assays' list)."},
	"batch_none":       {"err", "Select at least one eligible assay to run."},
	"blast_off":        {"err", "BLAST is not configured on this server (no NCBI email)."},
	"schedule_created": {"ok", "Schedule created."},
	"schedule_deleted": {"ok", "Schedule deleted."},
}

func flashFromQuery(r *http.Request) (text, kind string) {
	if f, ok := flashes[r.URL.Query().Get("msg")]; ok {
		return f.Text, f.Kind
	}
	return "", ""
}

func fmtTime(v any) string {
	const layout = "2006-01-02 15:04"
	switch t := v.(type) {
	case time.Time:
		if t.IsZero() {
			return ""
		}
		return t.Local().Format(layout)
	case *time.Time:
		if t == nil || t.IsZero() {
			return ""
		}
		return t.Local().Format(layout)
	default:
		return ""
	}
}
