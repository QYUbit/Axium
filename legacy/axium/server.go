package axium

import (
	"context"
	"fmt"
	"log"
	"sync"
)

type AxiumServer interface {
	ISessionManager
	IRoomManager
	OnConnect(func(*Session, string) (bool, string))
	OnDisconnect(func(*Session))
	OnShutdown(func())
	OnBeforeMessage(MiddlewareHandler)
	ActionHandler(byte, MessageHandler)
	MiddlewareHandler(MiddlewareHandler)
	MessageHandler(string, MessageHandler)
	CreateRoom(roomType string, id string) error
	Broadcast(data []byte, reliable bool) error
	BroadcastExcept(data []byte, reliable bool, exceptions ...string) error
	Shutdown() error
	Context() context.Context
}

type Server struct {
	*SessionManager
	*RoomManager

	transport   AxiumTransport
	serializer  AxiumSerializer
	idGenerator IdGenerator

	onConnect         func(*Session, string) (bool, string)
	onDisconnect      func(*Session)
	onShutdown        func()
	onError           func(error)
	onBeforeMessage   MiddlewareHandler
	actionHandlers    map[byte]MessageHandler
	actionHandlerMu   sync.RWMutex
	middlewareHandler MiddlewareHandler
	messageHandlers   map[string]MessageHandler
	messageHandlerMu  sync.RWMutex
	fallbackHandler   MessageHandler

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
		SessionManager:    sm,
		RoomManager:       rm,
		transport:         options.Transport,
		serializer:        options.Serializer,
		ctx:               ctx,
		cancel:            cancel,
		onConnect:         func(_ *Session, _ string) (bool, string) { return true, "" },
		onDisconnect:      func(s *Session) {},
		onShutdown:        func() {},
		onError:           func(err error) { log.Printf("An error occurred: %v", err) },
		actionHandlers:    make(map[byte]MessageHandler),
		onBeforeMessage:   func(session *Session, data []byte) bool { return true },
		middlewareHandler: func(session *Session, data []byte) bool { return true },
		messageHandlers:   make(map[string]MessageHandler),
		fallbackHandler:   func(session *Session, data []byte) {},
	}

	s.transport.OnConnect(s.handleConnect)
	s.transport.OnDisconnect(s.handleDisconnect)
	s.transport.OnMessage(s.handleMessage)
	s.transport.OnError(s.onError)

	return s
}

// TODO More idiomatic design (e.g. context)

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

// TODO Better server error handling

func (s *Server) handleMessage(sessionId string, data []byte) {
	session, err := s.getSession(sessionId)
	if err != nil {
		s.onError(err)
		return
	}

	if !s.onBeforeMessage(session, data) {
		return
	}

	msg, err := s.serializer.DecodeMessage(data)
	if err != nil {
		s.onError(fmt.Errorf("Failed to decode message: %v", err))
		return
	}

	switch msg.Action {
	case ServerEventAction:
		if !s.middlewareHandler(session, msg.Data) {
			return
		}

		s.messageHandlerMu.RLock()
		handler, exists := s.messageHandlers[msg.Event]
		s.messageHandlerMu.RUnlock()

		if !exists {
			s.fallbackHandler(session, msg.Data)
			return
		}

		handler(session, msg.Data)

	case RoomEventAction:
		room, err := s.getRoom(msg.RoomId)
		if err != nil {
			s.onError(err)
			return
		}

		if !room.middlewareHandler(session, msg.Data) {
			return
		}

		room.messageHandlerMu.RLock()
		handler, exists := room.messageHandlers[msg.Event]
		room.messageHandlerMu.RUnlock()

		if !exists {
			room.fallback(session, msg.Data)
			return
		}

		handler(session, msg.Data)

	default:
		s.actionHandlerMu.RLock()
		handler := s.actionHandlers[byte(msg.Action)]
		s.actionHandlerMu.RUnlock()

		handler(session, msg.Data)
	}
}

// ==============================================
// Event handlers
// ==============================================

func (s *Server) OnBeforeMessage(handler MiddlewareHandler) {
	s.onBeforeMessage = handler
}

func (s *Server) ActionHandler(action byte, handler MessageHandler) {
	if action < 10 {
		panic("actions 0 to 9 are reserved")
	}

	s.actionHandlerMu.Lock()
	s.actionHandlers[action] = handler
	s.actionHandlerMu.Unlock()
}

func (s *Server) MiddlewareHandler(handler MiddlewareHandler) {
	s.middlewareHandler = handler
}

func (s *Server) MessageHandler(eventType string, handler MessageHandler) {
	s.messageHandlerMu.Lock()
	s.messageHandlers[eventType] = handler
	s.messageHandlerMu.Unlock()
}

func (s *Server) FallbackHandler(handler MessageHandler) {
	s.fallbackHandler = handler
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

func (s *Server) OnError(fn func(error)) {
	s.onError = fn
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

func (s *Server) BroadcastExcept(data []byte, reliable bool, exceptions ...string) error {
	s.sessionMu.RLock()
	sendTo := make([]*Session, 0, len(s.sessions))

	excluded := make(map[string]struct{}, len(exceptions))
	for _, id := range exceptions {
		excluded[id] = struct{}{}
	}

	for id, session := range s.sessions {
		if _, skip := excluded[id]; !skip {
			sendTo = append(sendTo, session)
		}
	}
	s.sessionMu.RUnlock()

	var lastErr error
	for _, session := range sendTo {
		if err := session.Send(data, reliable); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// ==============================================
// Shutdown
// ==============================================

func (s *Server) Shutdown() error {
	var lastErr error

	s.cancel()

	s.onShutdown()

	if err := s.transport.Close(); err != nil {
		lastErr = err
	}

	roomIds := s.GetRoomIds()
	for _, roomId := range roomIds {
		if err := s.DestroyRoom(roomId); err != nil {
			lastErr = err
		}
	}

	sessionIds := s.GetSessionIds()
	for _, sessionId := range sessionIds {
		if err := s.DisconnectSession(sessionId, 1001, "Server shutdown"); err != nil { // code ?
			lastErr = err
		}
	}

	return lastErr
}

func (s *Server) Context() context.Context {
	return s.ctx
}
