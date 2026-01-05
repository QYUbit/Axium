package s2

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"
	"sync/atomic"
)

type Room struct {
	id string

	manager *roomManager
	router  *Router

	sessions   map[string]*Session
	sessionsMu sync.RWMutex

	onCreate  func(ctx context.Context)
	onJoin    func(s *Session)
	onLeave   func(s *Session)
	onDestroy func()

	cancel context.CancelFunc

	destroyOnce sync.Once
	isOpen      atomic.Bool
}

func newRoom(cancel context.CancelFunc, id string) *Room {
	r := &Room{
		id:       id,
		sessions: make(map[string]*Session),
		isOpen:   atomic.Bool{},
		cancel:   cancel,
	}
	r.isOpen.Store(true)
	return r
}

func (r *Room) tryIsOpen() error {
	if !r.isOpen.Load() {
		return errors.New("room closed")
	}
	return nil
}

func (r *Room) ID() string {
	return r.id
}

func (r *Room) MemberCount() int {
	r.sessionsMu.RLock()
	defer r.sessionsMu.RUnlock()
	return len(r.sessions)
}

func (r *Room) Members() []*Session {
	r.sessionsMu.RLock()
	defer r.sessionsMu.RUnlock()
	sessions := make([]*Session, 0, len(r.sessions))
	for _, member := range r.sessions {
		sessions = append(sessions, member)
	}
	return sessions
}

func (r *Room) MemberIDs() []string {
	r.sessionsMu.RLock()
	defer r.sessionsMu.RUnlock()
	sessions := make([]string, 0, len(r.sessions))
	for id := range r.sessions {
		sessions = append(sessions, id)
	}
	return sessions
}

func (r *Room) MembersIter() iter.Seq2[string, *Session] {
	return func(yield func(string, *Session) bool) {
		r.sessionsMu.RLock()
		defer r.sessionsMu.RUnlock()

		for id, s := range r.sessions {
			if !yield(id, s) {
				return
			}
		}
	}
}

func (r *Room) GetMember(sessionId string) (*Session, bool) {
	r.sessionsMu.RLock()
	defer r.sessionsMu.RUnlock()
	session, exists := r.sessions[sessionId]
	return session, exists
}

func (r *Room) HasMember(sessionId string) bool {
	r.sessionsMu.RLock()
	defer r.sessionsMu.RUnlock()
	_, exists := r.sessions[sessionId]
	return exists
}

func (r *Room) Assign(s *Session) {
	r.sessionsMu.Lock()
	r.sessions[s.ID()] = s
	r.sessionsMu.Unlock()

	s.addToRoom(r)

	r.onJoin(s)
}

func (r *Room) Unassign(s *Session) {
	r.sessionsMu.Lock()
	delete(r.sessions, s.ID())
	r.sessionsMu.Unlock()

	s.removeFromRoom(r)

	if r.isOpen.Load() {
		r.onLeave(s)
	}
}

func (r *Room) Destroy() {
	if swapped := r.isOpen.CompareAndSwap(true, false); !swapped {
		return
	}

	r.destroyOnce.Do(func() {
		r.cancel()

		sessions := r.Members()
		for _, s := range sessions {
			r.Unassign(s)
		}

		r.onDestroy()
	})
}

func (r *Room) buildMsg(path string, data []byte) Message {
	return Message{
		Path:   path,
		IsRoom: true,
		Room:   r.id,
		Data:   data,
	}
}

func (r *Room) Broadcast(path string, data []byte) error {
	msg := r.buildMsg(path, data)

	errorCount := 0
	var lastError error

	for _, s := range r.MembersIter() {
		if err := s.send(msg); err != nil {
			errorCount++
			lastError = err
		}
	}

	if errorCount == 0 {
		return nil
	}

	return fmt.Errorf("failed to broadcast to room %s. %d members failed: %v", r.id, errorCount, lastError)
}

func (r *Room) BroadcastExcept(path string, data []byte, exceptions []string) error {
	except := make(map[string]struct{}, len(exceptions))
	for _, id := range exceptions {
		except[id] = struct{}{}
	}

	included := make([]*Session, 0, len(r.sessions))
	for id, s := range r.MembersIter() {
		if _, ok := except[id]; !ok {
			included = append(included, s)
		}
	}

	msg := r.buildMsg(path, data)

	errorCount := 0
	var lastError error

	for _, s := range included {
		if err := s.send(msg); err != nil {
			errorCount++
			lastError = err
		}
	}

	if errorCount == 0 {
		return nil
	}

	return fmt.Errorf("failed to broadcast to room %s. %d members failed: %v", r.id, errorCount, lastError)
}

func (r *Room) SetRouter(router *Router) {
	r.router = router
}

func (r *Room) OnCreate(f func(ctx context.Context)) {
	r.onCreate = f
}

func (r *Room) OnJoin(f func(s *Session)) {
	r.onJoin = f
}

func (r *Room) OnLeave(f func(s *Session)) {
	r.onLeave = f
}

func (r *Room) OnDestroy(f func()) {
	r.onDestroy = f
}
