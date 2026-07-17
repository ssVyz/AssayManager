package auth

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

// Session is a logged-in user's server-side state. Stored in memory only, so
// all sessions are lost on restart (acceptable for the MVP).
type Session struct {
	ID        string
	UserID    int64
	CSRFToken string
	Created   time.Time
}

// Manager is a concurrency-safe in-memory session store.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	ttl      time.Duration
}

func NewManager(ttl time.Duration) *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		ttl:      ttl,
	}
}

// Create starts a new session for userID and returns it.
func (m *Manager) Create(userID int64) *Session {
	s := &Session{
		ID:        randToken(),
		UserID:    userID,
		CSRFToken: randToken(),
		Created:   time.Now(),
	}
	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()
	return s
}

// Get returns the session for id, if present and not expired.
func (m *Manager) Get(id string) (*Session, bool) {
	if id == "" {
		return nil, false
	}
	m.mu.RLock()
	s, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if m.ttl > 0 && time.Since(s.Created) > m.ttl {
		m.Destroy(id)
		return nil, false
	}
	return s, true
}

// Destroy removes a session (logout).
func (m *Manager) Destroy(id string) {
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
}

// randToken returns a URL-safe, cryptographically random token.
func randToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand should never fail; if it does, panicking is correct.
		panic("auth: cannot read random bytes: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
