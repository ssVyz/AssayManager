package web

import (
	"net/http"
	"strings"

	"AssayManager/internal/auth"
)

func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	if userFrom(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.render(w, http.StatusOK, "login", s.page(r, "", "Log in"))
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	user, err := s.store.UserByUsername(username)
	if err != nil || !auth.CheckPassword(user.PwHash, password) {
		http.Redirect(w, r, "/login?msg=badlogin", http.StatusSeeOther)
		return
	}

	sess := s.sessions.Create(user.ID)
	s.setSessionCookie(w, sess.ID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if sess := sessionFrom(r.Context()); sess != nil {
		s.sessions.Destroy(sess.ID)
	}
	s.clearSessionCookie(w)
	http.Redirect(w, r, "/login?msg=loggedout", http.StatusSeeOther)
}

func (s *Server) handleRegisterForm(w http.ResponseWriter, r *http.Request) {
	if userFrom(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.render(w, http.StatusOK, "register", s.page(r, "", "Register"))
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	password2 := r.FormValue("password2")

	if username == "" || password == "" {
		http.Redirect(w, r, "/register?msg=bad_register", http.StatusSeeOther)
		return
	}
	if password != password2 {
		http.Redirect(w, r, "/register?msg=pw_mismatch", http.StatusSeeOther)
		return
	}

	taken, err := s.store.UsernameTaken(username)
	if err != nil {
		s.serverError(w, "check username", err)
		return
	}
	if taken {
		http.Redirect(w, r, "/register?msg=user_taken", http.StatusSeeOther)
		return
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		s.serverError(w, "hash password", err)
		return
	}
	if _, err := s.store.CreateUser(username, hash); err != nil {
		s.serverError(w, "create user", err)
		return
	}
	http.Redirect(w, r, "/login?msg=registered", http.StatusSeeOther)
}

func (s *Server) setSessionCookie(w http.ResponseWriter, id string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		// Secure is intentionally off for local HTTP; enable once served over HTTPS.
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (s *Server) serverError(w http.ResponseWriter, what string, err error) {
	s.log.Error(what, "err", err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}
