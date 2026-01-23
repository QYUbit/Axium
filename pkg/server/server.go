// Package server manages sessions, orchestrates rooms and routes messages.
package server

import (
	"context"

	"github.com/QYUbit/Axium/pkg/axlog"
	"github.com/QYUbit/Axium/pkg/transport"
)

type Server struct {
	useUnreliable bool

	logger axlog.Logger

	transport transport.Transport

	connService *connService
}

type ServerConfig struct {
	Logger    axlog.Logger
	Transport transport.Transport
	Codec     Codec
}

func NewServer(cfg ServerConfig) *Server {
	connService := &connService{
		codec:  cfg.Codec,
		logger: cfg.Logger,
	}

	s := &Server{
		transport:   cfg.Transport,
		logger:      cfg.Logger,
		connService: connService,
	}

	return s
}

func (s *Server) Start(ctx context.Context) error {
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
		s.connService.Handle(ctx, p)
	}
}

func (s *Server) SetRouter(r *Router) {
	r.logger = s.logger
	s.connService.router = r
}
