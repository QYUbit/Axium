package axium

import (
	"fmt"
	"sync"
)

type ISessionManager interface {
	GetSession(sessionId string) (*Session, bool)
	QuerySession(key string, value any) []*Session
	QueryOneSession(key string, value any) (*Session, bool)
	QuerySessionFunc(fn func(store map[string]any) bool) []*Session
	QueryOneSessionFunc(fn func(store map[string]any) bool) (*Session, bool)
	GetSessions() []*Session
	GetSessionIds() []string
	SessionCount() int
	DisconnectSession(sessionId string, code int, reason string) error
}

type SessionManager struct {
	sessions  map[string]*Session
	sessionMu sync.RWMutex
}

func (sm *SessionManager) getSession(id string) (*Session, error) {
	sm.sessionMu.RLock()
	session, exists := sm.sessions[id]
	sm.sessionMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("session %s does not exist\n", id)
	}
	return session, nil
}

func (sm *SessionManager) setSession(id string, session *Session) {
	sm.sessionMu.Lock()
	sm.sessions[id] = session
	sm.sessionMu.Unlock()
}

func (sm *SessionManager) GetSession(sessionId string) (*Session, bool) {
	sm.sessionMu.RLock()
	defer sm.sessionMu.RUnlock()
	session, exists := sm.sessions[sessionId]
	return session, exists
}

func (sm *SessionManager) QuerySession(key string, value any) []*Session {
	sessions := sm.GetSessions()
	var filtered []*Session
	for _, s := range sessions {
		val, exists := s.Get(key)
		if exists && val == value {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

func (sm *SessionManager) QueryOneSession(key string, value any) (*Session, bool) {
	sessions := sm.GetSessions()
	for _, s := range sessions {
		val, exists := s.Get(key)
		if exists && val == value {
			return s, true
		}
	}
	return nil, false
}

func (sm *SessionManager) QuerySessionFunc(fn func(map[string]any) bool) []*Session {
	sessions := sm.GetSessions()
	var filtered []*Session
	for _, s := range sessions {
		store := s.GetAll()
		if fn(store) {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

func (sm *SessionManager) QueryOneSessionFunc(fn func(map[string]any) bool) (*Session, bool) {
	sessions := sm.GetSessions()
	for _, s := range sessions {
		store := s.GetAll()
		if fn(store) {
			return s, true
		}
	}
	return nil, false
}

func (sm *SessionManager) GetSessions() []*Session {
	sm.sessionMu.RLock()
	defer sm.sessionMu.RUnlock()
	sessions := make([]*Session, 0, len(sm.sessions))
	for _, session := range sm.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

func (sm *SessionManager) GetSessionIds() []string {
	sm.sessionMu.RLock()
	defer sm.sessionMu.RUnlock()
	ids := make([]string, 0, len(sm.sessions))
	for id := range sm.sessions {
		ids = append(ids, id)
	}
	return ids
}

func (sm *SessionManager) SessionCount() int {
	sm.sessionMu.RLock()
	defer sm.sessionMu.RUnlock()
	return len(sm.sessions)
}

func (sm *SessionManager) DisconnectSession(sessionId string, code int, reason string) error {
	sm.sessionMu.RLock()
	session, exists := sm.sessions[sessionId]
	sm.sessionMu.RUnlock()

	if !exists {
		return fmt.Errorf("session %s does not exist", sessionId)
	}

	return session.Close(code, reason)
}
