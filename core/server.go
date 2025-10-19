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

	onConnect        func(ip string) (bool, string)
	messageHandlers  map[string]MessageHandler
	messageHandlerMu sync.RWMutex

	roomDefinitions  map[string]RoomDefinition
	roomDefinitionMu sync.RWMutex
}

type ServerOptions struct {
	Transport  AxiumTransport
	Serializer AxiumSerializer
}

func NewServer(options ServerOptions) (*Server, error) {
	s := &Server{
		rooms: make(map[string]*Room),
	}

	s.transport = options.Transport
	s.serializer = options.Serializer

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

	pass, str := s.onConnect(ip)
	if pass {
		ctx := context.Background()
		session := NewSession(str, ip, ctx)

		s.sessionMu.Lock()
		s.sessions[str] = session
		s.sessionMu.Unlock()

		accept(str)
	} else {
		reject(str)
	}
}

func (s *Server) handleDisconnect(id string) {
	s.sessionMu.Lock()
	s.sessions[id].handleDisconnect()
	delete(s.sessions, id)
	s.sessionMu.Unlock()
}

func (s *Server) handleMessage(sessionId string, data []byte) {
	var msg Message
	if err := s.serializer.DecodeMessage(data, &msg); err != nil {
		fmt.Printf("Failed to decode message: %s\n", err)
		return
	}

	switch MessageAction(msg.MessageAction) {
	case RoomEventAction:
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
			fmt.Printf("handler for event type %s does not exist\n", msg.RoomEventMsg.EventType)
			return
		}

		handler(session, msg.RoomEventMsg.Data) // Do the same for server events
	case ServerEventAction:
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

func (s *Server) CreateRoom(typ string, id string) error {
	room := NewRoom(RoomConfig{
		Id:        id,
		Transport: s.transport,
		Context:   context.Background(),
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
	return nil
}

func (s *Server) DestroyRoom(roomId string) error {
	s.roomMu.RLock()
	room, exists := s.rooms[roomId]
	s.roomMu.RUnlock()

	if !exists {
		return fmt.Errorf("room %s does not exist\n", roomId)
	}

	s.roomMu.Lock()
	delete(s.rooms, roomId)
	s.roomMu.Unlock()

	return room.destroy()
}

func (s *Server) RegisterHandler(eventType string, handler MessageHandler) {
	s.messageHandlerMu.Lock()
	s.messageHandlers[eventType] = handler
	s.messageHandlerMu.Unlock()
}

func (s *Server) OnConnect(fn func(string) (bool, string)) {
	s.onConnect = fn
}

func (s *Server) DefineRoom(typ string, room RoomDefinition) {
	s.roomDefinitionMu.Lock()
	s.roomDefinitions[typ] = room
	s.roomDefinitionMu.Unlock()
}

func (s *Server) OnShutdown(fn func()) {

}
