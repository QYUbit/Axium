package s2

import (
	"context"
	"crypto/tls"
	"sync"
	"sync/atomic"

	"github.com/quic-go/quic-go"
)

type Server struct {
	addres  string
	quicCfg *quic.Config
	tlsCfg  *tls.Config

	listener    *quic.Listener
	protocol    MessageProtocol
	idGenerator func() string

	sessions *sessionManager
	rooms    *roomManager

	router *Router

	cancel    context.CancelFunc
	wg        sync.WaitGroup
	running   atomic.Bool
	closeOnce sync.Once
	startOnce sync.Once
}

func NewServer(addres string, tlsCfg *tls.Config, quicCfg *quic.Config, idGenerator func() string, protocol MessageProtocol) *Server {
	s := &Server{
		addres:      addres,
		tlsCfg:      tlsCfg,
		quicCfg:     quicCfg,
		idGenerator: idGenerator,
		rooms:       newRoomManager(idGenerator),
	}

	s.sessions = newSessionManager(idGenerator, s.dispatchMessage, protocol)

	return s
}

func (s *Server) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	l, err := quic.ListenAddr(s.addres, s.tlsCfg, s.quicCfg)
	if err != nil {
		return err
	}
	s.listener = l

	if err := s.acceptConnections(ctx); err != nil {
		return err
	}

	return nil
}

func (s *Server) acceptConnections(ctx context.Context) error {
	for {
		conn, err := s.listener.Accept(ctx)
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}

		go s.sessions.addSession(ctx, conn)
	}
}

func (s *Server) dispatchMessage(ctx context.Context, ses *Session, msg Message) {
	incoming := IncomingMessage{
		Sender: ses,
		Path:   msg.Path,
		Data:   msg.Data,
	}

	if !msg.IsRoom {
		if s.router != nil {
			s.router.trigger(ctx, incoming)
		}
		return
	}

	r, ok := s.rooms.getRoom(msg.Room)
	if !ok {
		// Log?
		return
	}

	if r.router != nil {
		r.router.trigger(ctx, incoming)
	}
}

func (s *Server) RegisterRoomFactory(typ string, f RoomFactory) {
	s.rooms.registerFactory(typ, f)
}

func (s *Server) CreateRoom(ctx context.Context, typ string) (*Room, error) {
	r, err := s.rooms.createRoom(ctx, typ)
	return r, err
}

func (s *Server) SetRouter(r *Router) {
	s.router = r
}

func (s *Server) Close() error {
	s.cancel()

	s.wg.Wait()

	return s.listener.Close()
}
