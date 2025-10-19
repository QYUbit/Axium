package core

import (
	"context"
	"sync"
)

type Session struct {
	id        string
	rooms     []*Room
	roomsMu   sync.RWMutex
	store     map[string]any
	ctx       context.Context
	cancel    context.CancelFunc
	transport AxiumTransport
}

func NewSession(id string, ip string, ctx context.Context) *Session {
	context, cancel := context.WithCancel(ctx)

	return &Session{
		id:     id,
		rooms:  make([]*Room, 0),
		ctx:    context,
		cancel: cancel,
	}
}

func (s *Session) Id() string {
	return s.id
}

func (s *Session) Get(key string) (value any, exists bool) {
	value, exists = s.store[key]
	return
}

func (s *Session) Set(key string, value any) {
	s.store[key] = value
}

func (s *Session) Send(data []byte, reliable bool) error {
	return s.transport.Send(s.id, data, reliable)
}

func (s *Session) Close(code int, reason string) error {
	s.cancel()
	s.handleDisconnect()
	return s.transport.CloseClient(s.id, code, reason)
}

func (s *Session) Context() context.Context {
	return s.ctx
}

func (s *Session) handleDisconnect() {
	s.roomsMu.Lock()
	for _, room := range s.rooms {
		_ = room
	}
	s.roomsMu.Unlock()
}
