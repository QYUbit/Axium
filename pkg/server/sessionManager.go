package server

import (
	"context"
	"io"
	"sync"

	"github.com/QYUbit/Axium/pkg/axlog"
	"github.com/QYUbit/Axium/pkg/transport"
)

// The sessionManager handles new connections and manages their lifetimes.
type sessionManager struct {
	router *Router
	codec  Codec
	logger axlog.Logger

	useUnreliable bool

	onConnect    func(ses *Session)
	onDisconnect func(ses *Session)

	sessions []*Session
	mu       sync.RWMutex
}

func (m *sessionManager) disconnectAll() (lastErr error) {
	var sessions []*Session
	m.mu.RLock()
	for _, ses := range m.sessions {
		sessions = append(sessions, ses)
	}
	m.mu.RUnlock()

	for _, ses := range m.sessions {
		if err := ses.Close(0, ""); err != nil { // TODO Code
			lastErr = err
		}
	}
	return
}

func (m *sessionManager) handle(ctx context.Context, peer transport.Peer) {
	ses := newSession(peer, m.codec)

	ses.wg.Go(func() {
		m.read(ctx, ses)
	})

	if m.useUnreliable {
		ses.wg.Go(func() {
			m.readUnreliable(ctx, ses)
		})
	}

	if m.onConnect != nil {
		m.onConnect(ses)
	}

	ses.wg.Wait()

	if err := ses.cleanup(); err != nil {
		m.logger.Error("failed to cleanup session", "error", err)
	}
}

func (m *sessionManager) read(ctx context.Context, ses *Session) {
	for {
		if ses.IsClosed() {
			break
		}

		msgs, err := m.codec.Read(ses.peer)
		if err != nil {
			if ctx.Err() != nil {
				m.logger.Debug("context cancelled, stopping reader")
				return
			}

			if err == io.EOF {
				m.logger.Debug("peer disconnected", "peer", ses.peer.RemoteAddr())
				return
			}

			m.logger.Error("failed to receive reliable message", "error", err)
			return
		}

		for _, msg := range msgs {
			m.router.Handle(ctx, ses, msg)
			if ses.IsClosed() {
				return
			}
		}
	}
}

func (m *sessionManager) readUnreliable(ctx context.Context, ses *Session) {
	for {
		data, err := ses.peer.ReceiveUnreliable()
		if err != nil {
			if ctx.Err() != nil {
				m.logger.Debug("context cancelled, stopping unreliable reader")
				return
			}

			if err == io.EOF {
				m.logger.Debug("peer disconnected (unreliable)")
				return
			}

			m.logger.Error("failed to receive unreliable message", "error", err)
			return
		}

		_ = data

		// TODO Handle unreliable
	}
}
