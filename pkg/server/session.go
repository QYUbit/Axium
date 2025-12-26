package server

import (
	"context"
	"sync"

	"github.com/QYUbit/Axium/pkg/transport"
)

type Session struct {
	id        string
	state     sync.Map
	rooms     []*Room
	roomsMu   sync.RWMutex
	server    *Server
	transport transport.Transport
	ctx       context.Context
	cancel    context.CancelFunc
}

type SessionConfig struct {
	ID        string
	Context   context.Context
	Server    *Server
	Transport transport.Transport
}

func NewSession(config SessionConfig) *Session {
	ctx, cancel := context.WithCancel(config.Context)

	return &Session{
		id:        config.ID,
		rooms:     make([]*Room, 0),
		server:    config.Server,
		transport: config.Transport,
		ctx:       ctx,
		cancel:    cancel,
	}
}

func (s *Session) ID() string {
	return s.id
}

func (s *Session) Context() context.Context {
	return s.ctx
}

func (s *Session) Rooms() []*Room {
	s.roomsMu.RLock()
	defer s.roomsMu.RUnlock()
	rooms := make([]*Room, len(s.rooms))
	copy(rooms, s.rooms)
	return rooms
}

func (s *Session) RoomCount() int {
	s.roomsMu.RLock()
	defer s.roomsMu.RUnlock()
	return len(s.rooms)
}

func (s *Session) IsInRoom(roomId string) bool {
	s.roomsMu.RLock()
	defer s.roomsMu.RUnlock()
	for _, room := range s.rooms {
		if room.id == roomId {
			return true
		}
	}
	return false
}

func (s *Session) GetRoom(roomId string) (*Room, bool) {
	s.roomsMu.RLock()
	defer s.roomsMu.RUnlock()
	for _, room := range s.rooms {
		if room.id == roomId {
			return room, true
		}
	}
	return nil, false
}

func (s *Session) addToRoom(room *Room) {
	s.roomsMu.Lock()
	s.rooms = append(s.rooms, room)
	s.roomsMu.Unlock()
}

func (s *Session) removeFromRoom(room *Room) {
	s.roomsMu.Lock()
	defer s.roomsMu.Unlock()

	for i, r := range s.rooms {
		if r.id == room.id {
			s.rooms = append(s.rooms[:i], s.rooms[i+1:]...)
			break
		}
	}
}

func (s *Session) JoinRoom(room *Room) {
	room.Assign(s)
}

func (s *Session) LeaveRoom(room *Room) {
	room.Unassign(s)
}

func (s *Session) LeaveRoomById(roomId string) {
	room, exists := s.GetRoom(roomId)
	if !exists {
		return
	}
	room.Unassign(s)
}

func (s *Session) LeaveAllRooms() {
	s.roomsMu.RLock()
	rooms := s.Rooms()
	for _, room := range rooms {
		room.Unassign(s)
	}
	s.roomsMu.RLock()
}

func (s *Session) Get(key string) (any, bool) {
	return s.state.Load(key)
}

func (s *Session) Set(key string, value any) {
	s.state.Store(key, value)
}

func (s *Session) Delete(key string) {
	s.state.Delete(key)
}

func (s *Session) Clear() {
	s.state.Clear()
}

func (s *Session) All(fn func(key, value any) bool) {
	s.state.Range(fn)
}

func (s *Session) Has(key string) bool {
	_, ok := s.state.Load(key)
	return ok
}

func (s *Session) buildMsg(path string, payload []byte) ([]byte, error) {
	msg := OutgoingMessage{
		Path:    path,
		Payload: payload,
	}
	var data []byte
	return data, s.server.protocol.Encode(data, msg)
}

func (s *Session) Send(path string, payload []byte) error {
	data, err := s.buildMsg(path, payload)
	if err != nil {
		return err
	}
	return s.transport.Send(s.id, data, true)
}

func (s *Session) SendUnreliable(path string, payload []byte) error {
	data, err := s.buildMsg(path, payload)
	if err != nil {
		return err
	}
	return s.transport.Send(s.id, data, false)
}

func (s *Session) Close(code int, reason string) error {
	return s.transport.CloseClient(s.id, code, reason)
}

func (s *Session) close() {
	s.cancel()
	s.LeaveAllRooms()
}
