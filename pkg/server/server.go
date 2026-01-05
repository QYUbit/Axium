package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QYUbit/Axium/pkg/transport"
)

type RoomFactory func(room *Room)

type Server struct {
	transport transport.Transport
	protocol  MessageProtocol

	roomFactories map[string]RoomFactory

	rooms   map[string]*Room
	roomsMu sync.RWMutex

	sessions   map[string]*Session
	sessionsMu sync.RWMutex

	router *Router

	idGenerator func() string

	cancel    context.CancelFunc
	wg        sync.WaitGroup
	running   atomic.Bool
	closeOnce sync.Once
}

func NewServer(transport transport.Transport) *Server {
	return &Server{
		transport:     transport,
		roomFactories: make(map[string]RoomFactory),
		rooms:         make(map[string]*Room),
		sessions:      make(map[string]*Session),
	}
}

func (s *Server) Start(ctx context.Context) error {
	if swapped := s.running.CompareAndSwap(false, true); !swapped {
		return ErrAlreadyRunning{}
	}

	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	if err := s.transport.Start(ctx); err != nil {
		return err
	}

	s.wg.Add(1)
	go s.connectionWorker(ctx)

	s.wg.Add(1)
	go s.messageWorker(ctx)

	return nil
}

func (s *Server) Close() error {
	if swapped := s.running.CompareAndSwap(true, false); !swapped {
		return ErrNotRunning{}
	}

	var lastError error

	s.closeOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}

		s.wg.Wait()

		lastError = s.transport.Close()
	})

	return lastError
}

func (s *Server) connectionWorker(ctx context.Context) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case conn := <-s.transport.Connections():
			s.sessionsMu.Lock()
			s.sessions[conn.ClientId] = NewSession(SessionConfig{
				ID:        conn.ClientId,
				Context:   ctx,
				Server:    s,
				Transport: s.transport,
			})
			s.sessionsMu.Unlock()
		case id := <-s.transport.Disconnections():
			s.sessionsMu.Lock()
			ses, ok := s.sessions[id]
			s.sessionsMu.Unlock()

			if ok {
				delete(s.sessions, id)
				ses.close()
			}
		}
	}
}

func (s *Server) messageWorker(ctx context.Context) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-s.transport.Messages():
			s.handleMessage(msg)
		}
	}
}

func (s *Server) handleMessage(raw transport.Message) {
	var msg IncomingMessage

	if err := s.protocol.Decode(&msg, raw); err != nil {
		return
	}

	ses, ok := s.getSession(msg.Sender)
	if !ok {
		return
	}

	var router *Router

	if msg.HasTarget {
		router = s.router
	} else {
		room, ok := s.getRoom(msg.Target)
		if ok {
			router = room.router
		}
	}

	if router != nil {
		router.trigger(ses, msg)
	}
}

func (s *Server) SetRouter(r *Router) {
	s.router = r
}

func (s *Server) SetProtocol(p MessageProtocol) {
	s.protocol = p
}

func (s *Server) RegisterRoom(name string, f RoomFactory) {
	s.roomFactories[name] = f
}

func (s *Server) CreateRoom(typ string) (*Room, error) {
	factory, ok := s.roomFactories[typ]
	if !ok {
		return nil, fmt.Errorf("Room factory \"%s\" not found", typ)
	}

	var id string
	if s.idGenerator != nil {
		id = s.idGenerator()
	} else {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			id = fmt.Sprintf("%d", time.Now().UnixNano())
		} else {
			id = hex.EncodeToString(b)
		}
	}

	room := NewRoom(RoomConfig{
		ID:        id,
		Server:    s,
		Transport: s.transport,
		Context:   context.Background(),
	})

	s.roomsMu.Lock()
	s.rooms[id] = room
	s.roomsMu.Unlock()

	factory(room)

	room.onCreate()

	return room, nil
}

func (s *Server) GetRoom(id string) (*Room, bool) {
	return s.getRoom(id)
}

func (s *Server) getRoom(id string) (*Room, bool) {
	s.roomsMu.Lock()
	defer s.roomsMu.Unlock()
	room, ok := s.rooms[id]
	return room, ok
}

func (s *Server) getSession(id string) (*Session, bool) {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	ses, ok := s.sessions[id]
	return ses, ok
}

func (s *Server) destroyRoom(r *Room) {
	s.roomsMu.Lock()
	delete(s.rooms, r.id)
	s.roomsMu.Unlock()
}
