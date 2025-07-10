package core

import (
	quichub "axium/quic-hub"
	"fmt"
	"sync"
)

type Server struct {
	hub    quichub.PubSubHub
	rooms  map[string]*Room
	roomMu sync.RWMutex
}

type ServerConfig struct {
	Host string
	Port int
}

func NewServer(config *ServerConfig) (*Server, error) {
	hub, err := quichub.NewQuicHub(fmt.Sprintf("%s:%d", config.Host, config.Port), nil, nil)
	if err != nil {
		return nil, err
	}

	s := &Server{
		hub:   hub,
		rooms: make(map[string]*Room),
	}

	hub.OnConnect(s.handleConnect)
	hub.OnMessage(s.handleMessage)

	return s, nil
}

func (s *Server) handleConnect(accept func(string), conn quichub.Connection) {

}

func (s *Server) handleMessage(clientId string, data []byte) {
	var msg Message
	if err := messageModel.Decode(data, &msg); err != nil {
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
