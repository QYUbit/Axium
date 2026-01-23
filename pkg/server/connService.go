package server

import (
	"context"

	"github.com/QYUbit/Axium/pkg/axlog"
	"github.com/QYUbit/Axium/pkg/transport"
)

type connService struct {
	useUnreliable bool
	router        *Router
	codec         Codec
	logger        axlog.Logger
}

func (h *connService) Handle(ctx context.Context, peer transport.Peer) {
	ses := newSession(peer, h.codec)

	go h.read(ctx, ses)
	if h.useUnreliable {
		go h.readUnreliable(ctx, ses)
	}

	<-ses.done
}

func (h *connService) read(ctx context.Context, ses *Session) {
	defer close(ses.done)

	for {
		msgs, err := h.codec.Read(ses.peer)
		if err != nil {
			select {
			case <-ctx.Done():
				break
			default:
				h.logger.Error("failed to receive reliable message", "error", err)
			}
		}

		for _, msg := range msgs {
			h.router.Handle(ctx, ses, msg)
		}
	}
}

func (h *connService) readUnreliable(ctx context.Context, ses *Session) {
	defer close(ses.done)

	for {
		data, err := ses.peer.ReceiveUnreliable()
		if err != nil {
			select {
			case <-ctx.Done():
				break
			default:
				h.logger.Error("failed to receive unreliable message", "error", err)
			}
		}

		_ = data

		// TODO Handle unreliable
	}
}
