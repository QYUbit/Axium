package s2

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
)

type Session struct {
	mainStreamTimeout time.Duration

	manager    *sessionManager
	protocol   MessageProtocol
	conn       *quic.Conn
	mainStream *quic.Stream

	id             string
	lastDatagram   []byte
	lastDatagramMu sync.Mutex

	rooms   map[string]*Room
	roomsMu sync.RWMutex

	cancel    context.CancelFunc
	closeOnce sync.Once
}

func newSession(
	cancel context.CancelFunc,
	id string,
	manager *sessionManager,
	protocol MessageProtocol,
	conn *quic.Conn,
) *Session {
	s := &Session{
		id:                id,
		manager:           manager,
		protocol:          protocol,
		conn:              conn,
		cancel:            cancel,
		rooms:             make(map[string]*Room),
		mainStreamTimeout: time.Second,
	}

	return s
}

func (s *Session) ID() string {
	return s.id
}

func (s *Session) Close(code quic.ApplicationErrorCode, reason string) error {
	var err error

	s.closeOnce.Do(func() {
		s.cancel()

		err = s.conn.CloseWithError(code, reason)

		s.leaveAllRooms()

		s.manager.removeSession(s.id)
	})

	return err
}

func (s *Session) streamLoop(ctx context.Context, dispatcher msgDispatcher) {
	for {
		stream, err := s.conn.AcceptStream(ctx)
		if err != nil {
			return
		}

		println("new stream")

		if s.mainStream == nil {
			s.mainStream = stream
			println("new main stream")
		}

		go s.readLoop(ctx, dispatcher, stream)
	}
}

func (s *Session) readLoop(ctx context.Context, dispatcher msgDispatcher, stream *quic.Stream) {
	for {
		var msg Message

		if err := s.protocol.ReadMessage(&msg, stream); err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				log.Println("Failed to read message:", err)
				// TODO Log
				continue
			}
		}

		fmt.Println("read message:", msg)

		dispatcher(ctx, s, msg)
	}
}

func (s *Session) datagramLoop(ctx context.Context) {
	for {
		data, err := s.conn.ReceiveDatagram(ctx)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				// TODO Log error
			}
		}

		s.lastDatagramMu.Lock()
		s.lastDatagram = data
		s.lastDatagramMu.Unlock()
	}
}

func (s *Session) Send(path string, data []byte) error {
	msg := Message{
		Path: path,
		Data: data,
	}
	return s.send(msg)
}

func (s *Session) send(msg Message) error {
	return s.protocol.WriteMessage(s.mainStream, msg)
}

func (s *Session) addToRoom(r *Room) {
	s.roomsMu.Lock()
	s.rooms[r.ID()] = r
	s.roomsMu.Unlock()
}

func (s *Session) removeFromRoom(r *Room) {
	s.roomsMu.Lock()
	delete(s.rooms, r.ID())
	s.roomsMu.Unlock()
}

func (s *Session) leaveAllRooms() {
	s.roomsMu.RLock()
	rooms := make([]*Room, 0, len(s.rooms))
	for _, r := range s.rooms {
		rooms = append(rooms, r)
	}
	s.roomsMu.RUnlock()

	for _, r := range s.rooms {
		r.Unassign(s)
	}
}
