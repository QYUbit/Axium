package core

import (
	"context"
	"fmt"
	"sync"
)

type AxiumConnection interface {
	Close(int, string)
	GetRemoteAddress() string
}

type AxiumTransport interface {
	CloseClient(string, int, string) error
	GetClientIds() []string
	Send(string, []byte, bool) error
	OnConnect(func(AxiumConnection, func(string), func(string)))
	OnDisconnect(func(string))
	OnMessage(func(string, []byte))
	Publish(string, []byte) error
	Subscribe(string, string) error
	Unsubscribe(string, string) error
	CreateTopic(string) error
	DeleteTopic(string) error
	GetTopicIds() []string
	GetClientIdsOfTopic(string) ([]string, error)
}

type AxiumSerializer interface {
	EncodeMessage(Message) ([]byte, error)
	DecodeMessage([]byte, *Message) error
}

type RoomDefinition func(*Room)

type Server struct {
	transport  AxiumTransport
	serializer AxiumSerializer

	sessions  map[string]*Session
	sessionMu sync.RWMutex
	rooms     map[string]*Room
	roomMu    sync.RWMutex

	onConnect        func(ip string) (pass bool, sessionId string, reason string)
	onDisconnect     func(*Session)
	onShutdown       func()
	messageHandlers  map[string]MessageHandler
	messageHandlerMu sync.RWMutex

	roomDefinitions  map[string]RoomDefinition
	roomDefinitionMu sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
}

type ServerOptions struct {
	Transport  AxiumTransport
	Serializer AxiumSerializer
}

func NewServer(options ServerOptions) (*Server, error) {
	ctx, cancel := context.WithCancel(context.Background())

	s := &Server{
		sessions:        make(map[string]*Session),
		rooms:           make(map[string]*Room),
		messageHandlers: make(map[string]MessageHandler),
		roomDefinitions: make(map[string]RoomDefinition),
		transport:       options.Transport,
		serializer:      options.Serializer,
		ctx:             ctx,
		cancel:          cancel,
		onConnect: func(ip string) (bool, string, string) {
			return true, "", ""
		},
		onDisconnect: func(s *Session) {},
		onShutdown:   func() {},
	}

	s.transport.OnConnect(s.handleConnect)
	s.transport.OnDisconnect(s.handleDisconnect)
	s.transport.OnMessage(s.handleMessage)

	return s, nil
}

// ==============================================
// Transport handler
// ==============================================

func (s *Server) handleConnect(conn AxiumConnection, accept func(string), reject func(string)) {
	ip := conn.GetRemoteAddress()

	pass, sessionId, reason := s.onConnect(ip) // new connect design
	if pass {
		session := NewSession(sessionId, s.ctx, s.transport)

		s.sessionMu.Lock()
		s.sessions[sessionId] = session
		s.sessionMu.Unlock()

		accept(sessionId)
	} else {
		reject(reason)
	}
}

func (s *Server) handleDisconnect(id string) {
	s.sessionMu.Lock()
	session, exists := s.sessions[id]
	if exists {
		session.handleDisconnect()
		delete(s.sessions, id)
	}
	s.sessionMu.Unlock()

	if exists {
		s.onDisconnect(session)
	}
}

func (s *Server) handleMessage(sessionId string, data []byte) {
	var msg Message
	if err := s.serializer.DecodeMessage(data, &msg); err != nil {
		fmt.Printf("Failed to decode message: %s\n", err)
		return
	}

	switch MessageAction(msg.MessageAction) {
	case RoomEventAction:
		if msg.RoomEventMsg == nil {
			fmt.Printf("RoomEventMsg is nil\n")
			return
		}

		s.roomMu.RLock()
		room, exists := s.rooms[msg.RoomEventMsg.RoomId]
		s.roomMu.RUnlock()

		if !exists {
			fmt.Printf("room %s does not exist\n", msg.RoomEventMsg.RoomId)
			return
		}

		s.sessionMu.RLock()
		session, exists := s.sessions[sessionId]
		s.sessionMu.RUnlock()

		if !exists {
			fmt.Printf("session %s does not exist\n", sessionId)
			return
		}

		room.messageHandlerMu.RLock()
		handler, exists := room.messageHandlers[msg.RoomEventMsg.EventType]
		room.messageHandlerMu.RUnlock()

		if !exists {
			room.fallback(session, msg.RoomEventMsg.Data)
			return
		}

		handler(session, msg.RoomEventMsg.Data)

	case ServerEventAction:
		if msg.ServerEventMsg == nil {
			fmt.Printf("ServerEventMsg is nil\n")
			return
		}

		s.sessionMu.RLock()
		session, exists := s.sessions[sessionId]
		s.sessionMu.RUnlock()

		if !exists {
			fmt.Printf("session %s does not exist\n", sessionId)
			return
		}

		s.messageHandlerMu.RLock()
		handler, exists := s.messageHandlers[msg.ServerEventMsg.EventType]
		s.messageHandlerMu.RUnlock()

		if !exists {
			fmt.Printf("handler for event type %s does not exist\n", msg.ServerEventMsg.EventType)
			return
		}

		handler(session, msg.ServerEventMsg.Data)
	}
}

// ==============================================
// Room management
// ==============================================

func (s *Server) CreateRoom(typ string, id string) error {
	room := NewRoom(RoomConfig{
		Id:         id,
		Transport:  s.transport,
		Serializer: s.serializer, // ?
		Context:    s.ctx,
	})

	s.roomDefinitionMu.RLock()
	definition, exists := s.roomDefinitions[typ]
	s.roomDefinitionMu.RUnlock()

	if !exists {
		return fmt.Errorf("room of type %s not found", typ)
	}

	definition(room)

	if err := s.transport.CreateTopic(id); err != nil {
		return err
	}

	s.roomMu.Lock()
	s.rooms[id] = room
	s.roomMu.Unlock()

	room.OnCreate()

	return nil
}

func (s *Server) DestroyRoom(roomId string) error {
	s.roomMu.Lock()
	room, exists := s.rooms[roomId]
	if exists {
		delete(s.rooms, roomId)
	}
	s.roomMu.Unlock()

	if !exists {
		return fmt.Errorf("room %s does not exist", roomId)
	}

	return room.destroy()
}

func (s *Server) GetRoom(roomId string) (*Room, bool) {
	s.roomMu.RLock()
	defer s.roomMu.RUnlock()
	room, exists := s.rooms[roomId]
	return room, exists
}

func (s *Server) GetRooms() []*Room {
	s.roomMu.RLock()
	defer s.roomMu.RUnlock()
	rooms := make([]*Room, 0, len(s.rooms))
	for _, room := range s.rooms {
		rooms = append(rooms, room)
	}
	return rooms
}

func (s *Server) GetRoomIds() []string {
	s.roomMu.RLock()
	defer s.roomMu.RUnlock()
	ids := make([]string, 0, len(s.rooms))
	for id := range s.rooms {
		ids = append(ids, id)
	}
	return ids
}

func (s *Server) DefineRoom(typ string, room RoomDefinition) {
	s.roomDefinitionMu.Lock()
	s.roomDefinitions[typ] = room
	s.roomDefinitionMu.Unlock()
}

// ==============================================
// Session management
// ==============================================

func (s *Server) GetSession(sessionId string) (*Session, bool) {
	s.sessionMu.RLock()
	defer s.sessionMu.RUnlock()
	session, exists := s.sessions[sessionId]
	return session, exists
}

func (s *Server) GetSessions() []*Session {
	s.sessionMu.RLock()
	defer s.sessionMu.RUnlock()
	sessions := make([]*Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

func (s *Server) GetSessionIds() []string {
	s.sessionMu.RLock()
	defer s.sessionMu.RUnlock()
	ids := make([]string, 0, len(s.sessions))
	for id := range s.sessions {
		ids = append(ids, id)
	}
	return ids
}

func (s *Server) SessionCount() int {
	s.sessionMu.RLock()
	defer s.sessionMu.RUnlock()
	return len(s.sessions)
}

func (s *Server) DisconnectSession(sessionId string, code int, reason string) error {
	s.sessionMu.RLock()
	session, exists := s.sessions[sessionId]
	s.sessionMu.RUnlock()

	if !exists {
		return fmt.Errorf("session %s does not exist", sessionId)
	}

	return session.Close(code, reason)
}

// ==============================================
// Event handlers
// ==============================================

func (s *Server) RegisterHandler(eventType string, handler MessageHandler) {
	s.messageHandlerMu.Lock()
	s.messageHandlers[eventType] = handler
	s.messageHandlerMu.Unlock()
}

func (s *Server) UnregisterHandler(eventType string) {
	s.messageHandlerMu.Lock()
	delete(s.messageHandlers, eventType)
	s.messageHandlerMu.Unlock()
}

func (s *Server) OnConnect(fn func(remoteAdress string) (pass bool, sessionId string, rejectReason string)) {
	s.onConnect = fn
}

func (s *Server) OnDisconnect(fn func(*Session)) {
	s.onDisconnect = fn
}

func (s *Server) OnShutdown(fn func()) {
	s.onShutdown = fn
}

// ==============================================
// Broadcasting
// ==============================================

func (s *Server) Broadcast(data []byte, reliable bool) error {
	s.sessionMu.RLock()
	sessions := make([]*Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	s.sessionMu.RUnlock()

	var lastErr error
	for _, session := range sessions {
		if err := session.Send(data, reliable); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (s *Server) BroadcastEvent(eventType string, data []byte, reliable bool) error {
	msg := Message{
		MessageAction: string(ServerEventAction),
		ServerEventMsg: &ServerEventMsg{
			EventType: eventType,
			Data:      data,
		},
	}

	encoded, err := s.serializer.EncodeMessage(msg)
	if err != nil {
		return err
	}

	return s.Broadcast(encoded, reliable)
}

// ==============================================
// Shutdown
// ==============================================

func (s *Server) Shutdown() error {
	s.cancel()

	s.onShutdown()

	roomIds := s.GetRoomIds()
	for _, roomId := range roomIds {
		if err := s.DestroyRoom(roomId); err != nil {
			fmt.Printf("Error destroying room %s: %s\n", roomId, err)
		}
	}

	sessionIds := s.GetSessionIds()
	for _, sessionId := range sessionIds {
		if err := s.DisconnectSession(sessionId, 1001, "Server shutdown"); err != nil { // code ?
			fmt.Printf("Error disconnecting session %s: %s\n", sessionId, err)
		}
	}

	return nil
}

func (s *Server) Context() context.Context {
	return s.ctx
}
