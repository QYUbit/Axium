package core

import (
	"fmt"
	"sync"
)

type AxiumConnection interface {
	Close(int, string)
	GetRemoteAddress() string
}

type AxiumHub interface {
	CloseClient(string, int, string) error
	GetClientIds() []string
	Send(string, []byte) error
	OnConnect(func(func(string), AxiumConnection))
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
	hub        AxiumHub
	serializer AxiumSerializer
	rooms      map[string]*Room
	roomMu     sync.RWMutex
}

type ServerOptions struct {
	Hub        AxiumHub
	Serializer AxiumSerializer
}

func NewServer(options ServerOptions) (*Server, error) {
	s := &Server{
		rooms: make(map[string]*Room),
	}

	s.hub = options.Hub
	s.serializer = options.Serializer

	s.hub.OnConnect(s.handleConnect)
	s.hub.OnDisconnect(s.handleDisconnect)
	s.hub.OnMessage(s.handleMessage)

	return s, nil
}

func (s *Server) handleConnect(accept func(string), conn AxiumConnection) {

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
