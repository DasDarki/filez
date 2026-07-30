// Package live holds ephemeral "live session" state: an in-memory map from a
// session id to the most recently pushed image. Nothing is persisted — a session
// only ever holds the latest frame, served live at /l/<id>.
package live

import (
	"sync"
	"time"

	"github.com/DasDarki/filez/internal/server/idgen"
)

// Session is one live broadcast holding its latest frame.
type Session struct {
	ID          string
	Name        string
	ContentType string
	Data        []byte
	Rev         int64 // increments on every push
	CreatedAt   int64
	UpdatedAt   int64
}

// Store is a concurrency-safe collection of live sessions.
type Store struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	maxImage int64
	now      func() int64
}

// New creates a Store; maxImage caps a single frame's size in bytes.
func New(maxImage int64) *Store {
	return &Store{
		sessions: make(map[string]*Session),
		maxImage: maxImage,
		now:      func() int64 { return time.Now().Unix() },
	}
}

// MaxImage returns the per-frame size cap.
func (s *Store) MaxImage() int64 { return s.maxImage }

// Create registers a new, empty session and returns it.
func (s *Store) Create() *Session {
	now := s.now()
	sess := &Session{ID: idgen.New(12), CreatedAt: now, UpdatedAt: now}
	s.mu.Lock()
	s.sessions[sess.ID] = sess
	s.mu.Unlock()
	return sess
}

// Put replaces a session's frame. ok is false if the session does not exist.
func (s *Store) Put(id, name, contentType string, data []byte) (rev int64, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.sessions[id]
	if sess == nil {
		return 0, false
	}
	sess.Name = name
	sess.ContentType = contentType
	sess.Data = data // replaced wholesale, so readers holding the old slice stay valid
	sess.Rev++
	sess.UpdatedAt = s.now()
	return sess.Rev, true
}

// Image returns a snapshot of the latest frame.
func (s *Store) Image(id string) (data []byte, contentType, name string, rev int64, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess := s.sessions[id]
	if sess == nil {
		return nil, "", "", 0, false
	}
	return sess.Data, sess.ContentType, sess.Name, sess.Rev, true
}

// Meta returns lightweight session info for polling.
func (s *Store) Meta(id string) (rev int64, name, contentType string, hasImage, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess := s.sessions[id]
	if sess == nil {
		return 0, "", "", false, false
	}
	return sess.Rev, sess.Name, sess.ContentType, len(sess.Data) > 0, true
}

// Delete ends a session.
func (s *Store) Delete(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

// Cleanup removes sessions not updated since idleBefore (unix seconds), returning
// the number removed.
func (s *Store) Cleanup(idleBefore int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for id, sess := range s.sessions {
		if sess.UpdatedAt <= idleBefore {
			delete(s.sessions, id)
			n++
		}
	}
	return n
}
