package server

import (
	"sync"
	"sync/atomic"
)

// The Room interface represents a group of sessions.
type Room interface {
	// ID retrives the id of a room.
	ID() string

	// Join adds a session to a room.
	Join(ses *Session) error

	// Leave removes a session from a room.
	Leave(ses *Session) error

	// Members retrieves all sessions in a room.
	Members() []*Session

	// MemberCount returns the number of sessions in a room.
	MemberCount() int

	// Close closes a room and removes all members.
	Close() error

	// IsClosed reports whther a room is closed.
	IsClosed() bool

	// Done returns a signal channel. This cannel will be
	// closed as soon as the room is closed.
	Done() <-chan struct{}
}

// BaseRoom implements Room interface.
type BaseRoom struct {
	serializer Serializer

	id       string
	sessions map[string]*Session
	mu       sync.RWMutex

	done      chan struct{}
	closed    atomic.Bool
	closeOnce sync.Once
}

// NewBaseRoom creates a new BaseRoom.
func NewBaseRoom(id string, serializer Serializer) *BaseRoom {
	return &BaseRoom{
		serializer: serializer,
		id:         id,
		sessions:   make(map[string]*Session),
		done:       make(chan struct{}),
	}
}

// ID implements Room.ID().
func (r *BaseRoom) ID() string {
	return r.id
}

// Join implements Room.Join().
func (r *BaseRoom) Join(ses *Session) error {
	if r.IsClosed() {
		return ErrRoomClosed
	}

	r.mu.Lock()
	r.sessions[ses.ID()] = ses
	r.mu.Unlock()

	return ses.addToRoom(r)
}

// Leave implements Room.Leave().
func (r *BaseRoom) Leave(ses *Session) error {
	if r.IsClosed() {
		return ErrRoomClosed
	}

	r.mu.Lock()
	delete(r.sessions, ses.ID())
	r.mu.Unlock()

	return ses.removeFromRoom(r)
}

// Members implements Room.Members().
func (r *BaseRoom) Members() []*Session {
	sessions := make([]*Session, 0, len(r.sessions))
	r.mu.RLock()
	for _, ses := range r.sessions {
		sessions = append(sessions, ses)
	}
	r.mu.RUnlock()
	return sessions
}

// MemberCount implements Room.MemberCount().
func (r *BaseRoom) MemberCount() int {
	r.mu.RLock()
	count := len(r.sessions)
	r.mu.RUnlock()
	return count
}

// Close implements Room.Close().
func (r *BaseRoom) Close() (lastErr error) {
	if ok := r.closed.CompareAndSwap(false, true); !ok {
		return ErrRoomClosed
	}

	for _, ses := range r.Members() {
		if err := r.Leave(ses); err != nil {
			lastErr = err
		}
	}

	close(r.done)

	return
}

// Done implements Room.Done().
func (r *BaseRoom) Done() <-chan struct{} {
	return r.done
}

// IsClosed implements Room.IsClosed().
func (r *BaseRoom) IsClosed() bool {
	return r.closed.Load()
}

// TODO Broadcast & Multicast

// Broadcast implements Room.Broadcast().
func (r *BaseRoom) Broadcast(path string, v any) (lastErr error) {
	if r.IsClosed() {
		return ErrRoomClosed
	}

	data, err := marshalPayload(r.serializer, v)
	if err != nil {
		return err
	}

	msg := Message{
		Path:   path,
		Data:   data,
		RoomID: r.id, // ?!
	}

	for _, ses := range r.sessions {
		if err := ses.send(msg); err != nil {
			lastErr = err
		}
	}

	return
}

// BroadcastExcept implements Room.BroadcastExcept().
func (r *BaseRoom) BroadcastExcept(path string, v any, except string) (lastErr error) {
	if r.IsClosed() {
		return ErrRoomClosed
	}

	data, err := marshalPayload(r.serializer, v)
	if err != nil {
		return err
	}

	msg := Message{
		Path:   path,
		Data:   data,
		RoomID: r.id, // ?!
	}

	for id, ses := range r.sessions {
		if id == except {
			continue
		}
		if err := ses.send(msg); err != nil {
			lastErr = err
		}
	}

	return
}
