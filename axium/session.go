package axium

import (
	"context"
	"fmt"
	"maps"
	"sync"
)

type ISession interface {
	Id() string
	Context() context.Context
	Rooms() []*Room
	RoomCount() int
	IsInRoom(roomId string) bool
	GetRoom(roomId string) (*Room, bool)
	Get(key string) (any, bool)
	Set(key string, value any)
	Delete(key string)
	GetAll() map[string]any
	Has(key string) bool
	Clear()
	Send(data []byte, reliable bool) error
	SendEvent(evenType string, data []byte, reliable bool, serializer AxiumSerializer) error // ?
	Close(code int, reason string) error
	JoinRoom(room *Room) error
	LeaveRoom(room *Room) error
	LeaveRoomById(roomId string) error
	LeaveAllRooms() error
}

type Session struct {
	id        string
	ip        string
	rooms     []*Room
	roomsMu   sync.RWMutex
	store     map[string]any
	storeMu   sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	transport AxiumTransport
}

func NewSession(id string, ctx context.Context, transport AxiumTransport) *Session {
	sessionCtx, cancel := context.WithCancel(ctx)

	return &Session{
		id:        id,
		rooms:     make([]*Room, 0),
		store:     make(map[string]any),
		ctx:       sessionCtx,
		cancel:    cancel,
		transport: transport,
	}
}

// ==============================================
// Getters
// ==============================================

func (s *Session) Id() string {
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

// ==============================================
// Store management
// ==============================================

func (s *Session) Get(key string) (value any, exists bool) {
	s.storeMu.RLock()
	defer s.storeMu.RUnlock()
	value, exists = s.store[key]
	return
}

func (s *Session) Set(key string, value any) {
	s.storeMu.Lock()
	defer s.storeMu.Unlock()
	s.store[key] = value
}

func (s *Session) Delete(key string) {
	s.storeMu.Lock()
	defer s.storeMu.Unlock()
	delete(s.store, key)
}

func (s *Session) GetAll() map[string]any {
	s.storeMu.RLock()
	defer s.storeMu.RUnlock()
	storeCopy := make(map[string]any, len(s.store))
	maps.Copy(storeCopy, s.store)
	return storeCopy
}

func (s *Session) Has(key string) bool {
	s.storeMu.RLock()
	defer s.storeMu.RUnlock()
	_, exists := s.store[key]
	return exists
}

func (s *Session) Clear() {
	s.storeMu.Lock()
	defer s.storeMu.Unlock()
	s.store = make(map[string]any)
}

// ==============================================
// Transport
// ==============================================

func (s *Session) Send(data []byte, reliable bool) error {
	return s.transport.Send(s.id, data, reliable)
}

func (s *Session) SendEvent(eventType string, data []byte, reliable bool, serializer AxiumSerializer) error {
	msg := Message{
		MessageAction: string(ServerEventAction),
		ServerEventMsg: &ServerEventMsg{
			EventType: eventType,
			Data:      data,
		},
	}

	encoded, err := serializer.EncodeMessage(msg)
	if err != nil {
		return err
	}

	return s.Send(encoded, reliable)
}

func (s *Session) Close(code int, reason string) error {
	s.cancel()
	s.handleDisconnect()
	return s.transport.CloseClient(s.id, code, reason)
}

// ==============================================
// Room management
// ==============================================

func (s *Session) joinRoom(room *Room) {
	s.roomsMu.Lock()
	defer s.roomsMu.Unlock()

	for _, r := range s.rooms {
		if r.id == room.id {
			return
		}
	}

	s.rooms = append(s.rooms, room)
}

func (s *Session) leaveRoom(room *Room) {
	s.roomsMu.Lock()
	defer s.roomsMu.Unlock()

	for i, r := range s.rooms {
		if r.id == room.id {
			s.rooms = append(s.rooms[:i], s.rooms[i+1:]...)
			break
		}
	}
}

func (s *Session) handleDisconnect() {
	s.roomsMu.Lock()
	rooms := make([]*Room, len(s.rooms))
	copy(rooms, s.rooms)
	s.roomsMu.Unlock()

	for _, room := range rooms {
		if err := room.Unassign(s); err != nil {
			fmt.Printf("Error unassigning session %s from room %s: %s\n", s.id, room.id, err)
		}
	}
}

// ==============================================
// Room operations
// ==============================================

func (s *Session) JoinRoom(room *Room) error {
	return room.Assign(s)
}

func (s *Session) LeaveRoom(room *Room) error {
	return room.Unassign(s)
}

func (s *Session) LeaveRoomById(roomId string) error {
	room, exists := s.GetRoom(roomId)
	if !exists {
		return fmt.Errorf("session %s is not in room %s", s.id, roomId)
	}
	return room.Unassign(s)
}

func (s *Session) LeaveAllRooms() error {
	rooms := s.Rooms()
	var lastErr error
	for _, room := range rooms {
		if err := room.Unassign(s); err != nil {
			lastErr = err
		}
	}
	return lastErr
}
