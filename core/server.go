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

type IdGenerator func() string

type MessageHandler func(origin *Session, data []byte)

type RoomDefinition func(*Room)

type Server struct {
	*SessionManager
	*RoomManager

	transport   AxiumTransport
	serializer  AxiumSerializer
	idGenerator IdGenerator

	onConnect        func(*Session, string) (bool, string)
	onDisconnect     func(*Session)
	onShutdown       func()
	messageHandlers  map[string]MessageHandler
	messageHandlerMu sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
}

type ServerOptions struct {
	Transport   AxiumTransport
	Serializer  AxiumSerializer
	IdGenerator IdGenerator
}

func NewServer(options ServerOptions) (*Server, error) {
	ctx, cancel := context.WithCancel(context.Background())

	sm := &SessionManager{
		sessions: make(map[string]*Session),
	}

	rm := &RoomManager{
		rooms:           make(map[string]*Room),
		roomDefinitions: make(map[string]RoomDefinition),
	}

	s := &Server{
		SessionManager:  sm,
		RoomManager:     rm,
		messageHandlers: make(map[string]MessageHandler),
		transport:       options.Transport,
		serializer:      options.Serializer,
		ctx:             ctx,
		cancel:          cancel,
		onConnect:       func(_ *Session, _ string) (bool, string) { return true, "" },
		onDisconnect:    func(s *Session) {},
		onShutdown:      func() {},
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

	sessionId := s.idGenerator()
	session := NewSession(sessionId, s.ctx, s.transport)

	pass, reason := s.onConnect(session, ip)

	if pass {
		s.setSession(sessionId, session)
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

		room, err := s.getRoom(msg.RoomEventMsg.RoomId)
		if err != nil {
			fmt.Printf("%v\n", err)
			return
		}

		session, err := s.getSession(sessionId)
		if err != nil {
			fmt.Printf("%v\n", err)
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

		session, err := s.getSession(sessionId)
		if err != nil {
			fmt.Printf("%v\n", err)
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

func (s *Server) OnConnect(fn func(session *Session, ip string) (pass bool, rejectReason string)) {
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
	sessions := s.GetSessions()

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

// ==============================================
// Session Manager
// ==============================================

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

// ==============================================
// Room Manager
// ==============================================

type RoomManager struct {
	rooms            map[string]*Room
	roomMu           sync.RWMutex
	roomDefinitions  map[string]RoomDefinition
	roomDefinitionMu sync.RWMutex
}

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

	s.setRooom(id, room)

	room.OnCreate()

	return nil
}

func (rm *RoomManager) DestroyRoom(roomId string) error {
	rm.roomMu.Lock()
	room, exists := rm.rooms[roomId]
	if exists {
		delete(rm.rooms, roomId)
	}
	rm.roomMu.Unlock()

	if !exists {
		return fmt.Errorf("room %s does not exist", roomId)
	}

	return room.destroy()
}

func (rm *RoomManager) getRoom(id string) (*Room, error) {
	rm.roomMu.RLock()
	room, exists := rm.rooms[id]
	rm.roomMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("room %s does not exist\n", id)
	}
	return room, nil
}

func (rm *RoomManager) setRooom(id string, room *Room) {
	rm.roomMu.Lock()
	rm.rooms[id] = room
	rm.roomMu.Unlock()
}

func (rm *RoomManager) GetRoom(roomId string) (*Room, bool) {
	rm.roomMu.RLock()
	defer rm.roomMu.RUnlock()
	room, exists := rm.rooms[roomId]
	return room, exists
}

func (rm *RoomManager) GetRooms() []*Room {
	rm.roomMu.RLock()
	defer rm.roomMu.RUnlock()
	rooms := make([]*Room, 0, len(rm.rooms))
	for _, room := range rm.rooms {
		rooms = append(rooms, room)
	}
	return rooms
}

func (rm *RoomManager) GetRoomIds() []string {
	rm.roomMu.RLock()
	defer rm.roomMu.RUnlock()
	ids := make([]string, 0, len(rm.rooms))
	for id := range rm.rooms {
		ids = append(ids, id)
	}
	return ids
}

func (rm *RoomManager) DefineRoom(typ string, room RoomDefinition) {
	rm.roomDefinitionMu.Lock()
	rm.roomDefinitions[typ] = room
	rm.roomDefinitionMu.Unlock()
}
