package web

import (
	"context"
	"net/http"
	"time"

	"AssayManager/internal/auth"
	"AssayManager/internal/store"
)

const cookieName = "am_session"

type ctxKey int

const (
	ctxUser ctxKey = iota
	ctxSession
)

func userFrom(ctx context.Context) *store.User {
	u, _ := ctx.Value(ctxUser).(*store.User)
	return u
}

func sessionFrom(ctx context.Context) *auth.Session {
	s, _ := ctx.Value(ctxSession).(*auth.Session)
	return s
}

// base loads any session, recovers from panics, and logs requests. It wraps the
// whole mux.
func (s *Server) base(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("panic recovered", "path", r.URL.Path, "err", rec)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()

		ctx := r.Context()
		if c, err := r.Cookie(cookieName); err == nil {
			if sess, ok := s.sessions.Get(c.Value); ok {
				if u, err := s.store.UserByID(sess.UserID); err == nil {
					ctx = context.WithValue(ctx, ctxUser, &u)
					ctx = context.WithValue(ctx, ctxSession, sess)
				}
			}
		}
		r = r.WithContext(ctx)

		start := time.Now()
		next.ServeHTTP(w, r)
		s.log.Info("request", "method", r.Method, "path", r.URL.Path, "dur", time.Since(start))
	})
}

// protected requires an authenticated user and, for unsafe methods, a valid
// CSRF token, capping the request body at the normal (small) form limit.
func (s *Server) protected(h http.HandlerFunc) http.HandlerFunc {
	return s.protectedN(s.cfg.MaxUploadBytes, h)
}

// protectedUpload is like protected but allows a much larger body, for routes
// that accept a file upload (the reference FASTA).
func (s *Server) protectedUpload(h http.HandlerFunc) http.HandlerFunc {
	return s.protectedN(s.cfg.MaxReferenceUploadBytes, h)
}

func (s *Server) protectedN(maxBytes int64, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := userFrom(r.Context())
		if user == nil {
			http.Redirect(w, r, "/login?msg=login_required", http.StatusSeeOther)
			return
		}
		if !safeMethod(r.Method) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			sess := sessionFrom(r.Context())
			if sess == nil || r.FormValue("csrf_token") != sess.CSRFToken {
				http.Error(w, "invalid or missing CSRF token", http.StatusForbidden)
				return
			}
		}
		h(w, r)
	}
}

func safeMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}
