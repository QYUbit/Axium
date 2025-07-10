package core

import "github.com/google/uuid"

type RoomMessage struct {
	Event  uint32
	Origin string
	Data   []byte
}

type Room struct {
	id           string
	server       *Server
	tickInterval int
	onMessage    func(RoomMessage)
}

type RoomConfig struct {
}

func (s *Server) CreateRoom(config *RoomConfig) (*Room, error) {
	id := uuid.New().String()

	r := &Room{
		id:     id,
		server: s,
	}

	if err := s.hub.CreateTopic(id); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *Room) OnMessage(fn func(RoomMessage)) {
	r.onMessage = fn
}
