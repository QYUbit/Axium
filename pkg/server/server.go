// Package server manages sessions, orchestrates rooms and routes messages.
package server

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/QYUbit/Axium/pkg/axlog"
	"github.com/QYUbit/Axium/pkg/transport"
)

// The Server struct is the central piece of the server package.
// It manages sessions, orchestrates rooms and routes messages.
type Server struct {
	logger    axlog.Logger
	transport transport.Transport

	sessionManager *sessionManager
	roomManager    *roomManager

	useUnreliable bool

	started   atomic.Bool
	closeOnce sync.Once
	closed    atomic.Bool
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

type ServerConfig struct {
	Logger    axlog.Logger
	Transport transport.Transport
	Codec     Codec
}

// NewServer creates a new Server.
func NewServer(cfg ServerConfig) *Server {
	sessionManager := &sessionManager{
		codec:  cfg.Codec,
		logger: cfg.Logger,
	}

	roomManager := newRoomManager()

	s := &Server{
		transport:      cfg.Transport,
		logger:         cfg.Logger,
		sessionManager: sessionManager,
		roomManager:    roomManager,
	}

	return s
}

// Start starts the server and accepts incomming connections.
func (s *Server) Start(ctx context.Context) error {
	if ok := s.started.CompareAndSwap(false, true); !ok {
		return ErrAlreadyStarted
	}
	if s.IsClosed() {
		return ErrServerClosed
	}

	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	defer s.cancel()

	for {
		p, err := s.transport.Accept(ctx)
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				s.logger.Error("failed to accept new peer", "error", err)
			}
		}

		s.wg.Go(func() {
			s.sessionManager.handle(ctx, p)
		})
	}
}

// IsClosed reports whether the server is closed.
func (s *Server) IsClosed() bool {
	return s.closed.Load()
}

// Close closes the server and releases it's resources.
func (s *Server) Close() (lastErr error) {
	if s.started.Load() {
		return ErrServerNotRunning
	}

	s.closeOnce.Do(func() {
		s.closed.Store(true)

		if s.cancel != nil {
			s.cancel()
		}

		if err := s.transport.Close(); err != nil {
			lastErr = err
		}
	})
	return
}

// Shutdown closes the server gracefully.
func (s *Server) Shutdown() (lastErr error) {
	if s.started.Load() {
		return ErrServerNotRunning
	}

	s.closeOnce.Do(func() {
		s.closed.Store(true)

		if s.cancel != nil {
			s.cancel()
		}

		if err := s.transport.Close(); err != nil {
			lastErr = err
		}

		if err := s.sessionManager.disconnectAll(); err != nil {
			lastErr = err
		}
	})
	return
}

func (s *Server) CreateRoom(r Room) {
	s.roomManager.set(r)
}

func (s *Server) GetRoom(id string) (Room, bool) {
	return s.roomManager.get(id)
}

func (s *Server) GetSession(id string) {

}

func (s *Server) SetOnConnect(f func(ses *Session)) {
	s.sessionManager.onConnect = f
}

func (s *Server) SetOnDisconnect(f func(ses *Session)) {
	s.sessionManager.onDisconnect = f
}

func (s *Server) SetRouter(r *Router) {
	r.logger = s.logger
	s.sessionManager.router = r
}
