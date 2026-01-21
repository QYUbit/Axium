package frame

import (
	"sync"

	"github.com/QYUbit/Axium/pkg/transport"
)

type Session struct {
	codec Codec
	peer  transport.Peer
	data  sync.Map
	done  chan struct{}
}

func newSession(peer transport.Peer, codec Codec) *Session {
	return &Session{
		peer:  peer,
		codec: codec,
	}
}

func (s *Session) Set(key string, value any) {
	s.data.Store(key, value)
}

func (s *Session) Get(key string) (any, bool) {
	return s.data.Load(key)
}

func (s *Session) Has(key string) bool {
	_, ok := s.data.Load(key)
	return ok
}

func (s *Session) Delete(key string) {
	s.data.Delete(key)
}

func (s *Session) Push(route string, payload []byte) error {
	msg := Message{
		ReqID: 0,
		Path:  route,
		Data:  payload,
	}

	return s.send(msg)
}

// TODO Use channel for backpressure

func (s *Session) send(msg Message) error {
	return s.codec.Encode(s.peer, msg)
}

func (s *Session) Kick(reason string) error {
	return s.peer.Close(transport.CloseCode(0), reason) // TODO Close code
}
