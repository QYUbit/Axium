package axium

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

type AxiumServer interface {
	ISessionManager
	IRoomManager
	OnConnect(func(*Session, string) (bool, string))
	OnDisconnect(func(*Session))
	OnShutdown(func())
	RegisterHandler(string, MessageHandler)
	UnregisterHandler(string)
	CreateRoom(roomType string, id string) error
	Broadcast(data []byte, reliable bool) error
	BroadcastEvent(eventType string, data []byte, reliable bool) error
	Shutdown() error
	Context() context.Context
}

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

func NewServer(options ServerOptions) AxiumServer {
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

	return s
}

type extendedServerOptions struct {
	Transport      AxiumTransport
	Serializer     AxiumSerializer
	SessionManager *SessionManager
	RoomManager    *RoomManager
	IdGenerator    IdGenerator
	Context        context.Context
}

func newServer(options extendedServerOptions) *Server {
	ctx, cancel := context.WithCancel(options.Context)

	s := &Server{
		SessionManager:  options.SessionManager,
		RoomManager:     options.RoomManager,
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

	return s
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

func (s *Server) RegisterMiddleware() {

}

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
// Create room
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

	s.setRooom(id, room)

	room.onCreate()

	return nil
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
