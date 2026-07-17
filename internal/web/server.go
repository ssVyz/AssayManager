// Package web is the HTTP layer: routing, middleware, handlers, and templates.
// Handlers stay thin and delegate to the store/analysis packages, so the same
// business logic could later back a JSON API instead of server-rendered HTML.
package web

import (
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"

	"AssayManager/internal/analysis"
	"AssayManager/internal/auth"
	"AssayManager/internal/config"
	"AssayManager/internal/store"
)

type Server struct {
	cfg      config.Config
	log      *slog.Logger
	store    *store.Store
	sessions *auth.Manager
	analyzer analysis.Analyzer
	tmpl     map[string]*template.Template
}

func New(cfg config.Config, log *slog.Logger, st *store.Store, sessions *auth.Manager, analyzer analysis.Analyzer) (*Server, error) {
	tmpl, err := parseTemplates()
	if err != nil {
		return nil, err
	}
	return &Server{
		cfg:      cfg,
		log:      log,
		store:    st,
		sessions: sessions,
		analyzer: analyzer,
		tmpl:     tmpl,
	}, nil
}

// Handler returns the fully-wired HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticSub)))

	// Public.
	mux.HandleFunc("GET /login", s.handleLoginForm)
	mux.HandleFunc("POST /login", s.handleLogin)
	mux.HandleFunc("GET /register", s.handleRegisterForm)
	mux.HandleFunc("POST /register", s.handleRegister)

	// Authenticated.
	mux.HandleFunc("POST /logout", s.protected(s.handleLogout))
	mux.HandleFunc("GET /{$}", s.protected(s.handleDashboard))
	mux.HandleFunc("GET /profile", s.protected(s.handleProfile))
	mux.HandleFunc("POST /profile", s.protected(s.handleProfileSave))

	mux.HandleFunc("GET /assays", s.protected(s.handleAssaysList))
	mux.HandleFunc("GET /assays/new", s.protected(s.handleAssayNew))
	mux.HandleFunc("POST /assays/preview", s.protected(s.handleAssayPreview))
	mux.HandleFunc("POST /assays", s.protected(s.handleAssaySave))
	mux.HandleFunc("GET /assays/history", s.protected(s.handleAssayHistory))
	mux.HandleFunc("POST /assays/delete", s.protected(s.handleAssayDelete))
	mux.HandleFunc("GET /assays/{id}", s.protected(s.handleAssayView))
	mux.HandleFunc("GET /assays/{id}/edit", s.protected(s.handleAssayEdit))

	mux.HandleFunc("GET /run", s.protected(s.handleRunForm))
	mux.HandleFunc("POST /run", s.protected(s.handleRunStart))
	mux.HandleFunc("GET /scheduled", s.protected(s.handleScheduled))
	mux.HandleFunc("GET /results", s.protected(s.handleResultsList))
	mux.HandleFunc("GET /results/{id}", s.protected(s.handleResultView))

	return s.base(mux)
}
