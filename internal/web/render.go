package web

import (
	"bytes"
	"embed"
	"html/template"
	"net/http"
	"time"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// pages are the content templates; each is parsed together with the layout.
var pages = []string{
	"login", "register", "dashboard", "profile",
	"assays_list", "assay_form", "assay_view", "assay_history",
	"run", "scheduled", "results_list", "result_view",
}

func parseTemplates() (map[string]*template.Template, error) {
	funcs := template.FuncMap{"fmtTime": fmtTime}
	m := make(map[string]*template.Template, len(pages))
	for _, p := range pages {
		t, err := template.New("layout.html").Funcs(funcs).
			ParseFS(templateFS, "templates/layout.html", "templates/"+p+".html")
		if err != nil {
			return nil, err
		}
		m[p] = t
	}
	return m, nil
}

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
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout.html", pd); err != nil {
		s.log.Error("render failed", "page", page, "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

var flashes = map[string]struct{ Kind, Text string }{
	"registered":     {"ok", "Account created — please log in."},
	"loggedout":      {"ok", "You have been logged out."},
	"profile_saved":  {"ok", "Profile updated."},
	"assay_saved":    {"ok", "Assay version saved."},
	"assay_deleted":  {"ok", "Assay deleted."},
	"run_started":    {"ok", "Analysis run started — see Check results."},
	"badlogin":       {"err", "Invalid username or password."},
	"login_required": {"err", "Please log in to continue."},
	"pw_mismatch":    {"err", "Passwords do not match."},
	"pw_empty":       {"err", "Password must not be empty."},
	"user_taken":     {"err", "That username is already taken."},
	"bad_register":   {"err", "Provide a username and matching, non-empty passwords."},
	"not_found":      {"err", "That item was not found."},
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
