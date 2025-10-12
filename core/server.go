package core

import (
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

type Server struct {
	transport  AxiumTransport
	serializer AxiumSerializer
	rooms      map[string]Room
	roomMu     sync.RWMutex
	onConnect  func()
}

type ServerOptions struct {
	Transport  AxiumTransport
	Serializer AxiumSerializer
}

func NewServer(options ServerOptions) (*Server, error) {
	s := &Server{
		rooms: make(map[string]Room),
	}

	s.transport = options.Transport
	s.serializer = options.Serializer

	s.transport.OnConnect(s.handleConnect)
	s.transport.OnDisconnect(s.handleDisconnect)
	s.transport.OnMessage(s.handleMessage)

	return s, nil
}

func (s *Server) handleConnect(conn AxiumConnection, accept func(string), reject func(string)) {

}

func (s *Server) handleDisconnect(string) {

}

func (s *Server) handleMessage(clientId string, data []byte) {
	var msg Message
	if err := s.serializer.DecodeMessage(data, &msg); err != nil {
		fmt.Printf("Failed to decode message: %s\n", err)
		return
	}

	switch MessageAction(msg.MessageAction) {
	case RoomEventAction:
		s.roomMu.RLock()
		room, exists := s.rooms[msg.RoomEvent.RoomId]
		s.roomMu.RUnlock()

		if !exists {
			fmt.Printf("room %s does not exist\n", msg.RoomEvent.RoomId)
			return
		}

		room.onMessage(RoomMessage{
			Event:  msg.RoomEvent.EventType,
			Origin: clientId,
			Data:   msg.RoomEvent.Data,
		})
	}
}
