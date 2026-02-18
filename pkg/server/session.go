package server

import (
	"sync"
	"sync/atomic"

	"github.com/QYUbit/Axium/pkg/transport"
)

// The Session struct wrapps a low level peer. It manages associated data
// such as an id, rooms and a session store (hash map).
type Session struct {
	codec      Codec
	serializer Serializer
	peer       transport.Peer
	manager    *sessionManager

	id    string
	data  sync.Map
	rooms sync.Map

	closeOnce sync.Once
	closed    atomic.Bool
	wg        sync.WaitGroup
}

func newSession(peer transport.Peer, codec Codec) *Session {
	return &Session{
		peer:  peer,
		codec: codec,
	}
}

// ==================================================================
// Lifecycle
// ==================================================================

// Close closes the low level peer of a session which will result in the cleanup of the session.
func (s *Session) Close(code transport.CloseCode, reason string) (err error) {
	s.closeOnce.Do(func() {
		s.closed.Store(true)

		err = s.peer.Close(code, reason)
	})
	return
}

// IsClosed reports whether a session is closed.
func (s *Session) IsClosed() bool {
	return s.closed.Load()
}

func (s *Session) leaveAllRooms() (lastErr error) {
	s.rooms.Range(func(key, value any) bool {
		r, ok := value.(Room)
		if !ok {
			return true
		}

		if err := r.Leave(s); err != nil {
			lastErr = err
		}
		return true
	})
	return
}

func (s *Session) cleanup() (lastErr error) {
	lastErr = s.Close(transport.CloseCode(0), "")
	if err := s.leaveAllRooms(); err != nil {
		lastErr = err
	}
	if s.manager.onDisconnect != nil {
		s.manager.onDisconnect(s)
	}
	return
}

// ==================================================================
// ID
// ==================================================================

// ID retrieves the id of a session.
func (s *Session) ID() string {
	return s.id
}

// SetID sets the id of a session.
func (s *Session) SetID(id string) {
	s.id = id
}

// ==================================================================
// Low level
// ==================================================================

// Peer returns the session's low level peer.
func (s *Session) Peer() transport.Peer {
	return s.peer
}

// ==================================================================
// Send
// ==================================================================

// Push sends a message to the session's peer. v can be a byte slice, a string
// or any value that can be serialized by the serializer.
func (s *Session) Push(route string, v any) error {
	data, err := marshalPayload(s.serializer, v)
	if err != nil {
		return err
	}

	msg := Message{
		ReqID: 0,
		Path:  route,
		Data:  data,
	}

	return s.send(msg)
}

// TODO Use channel for backpressure or just SetDeadline, idk

func (s *Session) send(msg Message) error {
	if s.IsClosed() {
		return ErrSessionClosed
	}
	return s.codec.Write(s.peer, msg)
}

// ==================================================================
// State
// ==================================================================

// Set sets a value in the session's store.
func (s *Session) Set(key string, value any) {
	s.data.Store(key, value)
}

// Get retrieves a value from the session's store.
func (s *Session) Get(key string) (any, bool) {
	return s.data.Load(key)
}

// Has reports whether the session's store contains the specified key.
func (s *Session) Has(key string) bool {
	_, ok := s.data.Load(key)
	return ok
}

// Delete deletes a key value pair form the session's store.
func (s *Session) Delete(key string) {
	s.data.Delete(key)
}

// Clear cleares the session's store.
func (s *Session) Clear() {
	s.data.Clear()
}

// ==================================================================
// Rooms
// ==================================================================

func (s *Session) addToRoom(r Room) error {
	if s.IsClosed() {
		return ErrSessionClosed
	}
	s.rooms.Store(r.ID(), r)
	return nil
}

func (s *Session) removeFromRoom(r Room) error {
	s.rooms.Delete(r.ID())
	return nil
}
