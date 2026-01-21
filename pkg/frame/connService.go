package frame

import (
	"context"

	"github.com/QYUbit/Axium/pkg/transport"
)

type connService struct {
	useUnreliable bool
	msgService    *messageService
	codec         Codec
}

func (h *connService) Handle(ctx context.Context, peer transport.Peer) {
	ses := newSession(peer)

	go h.read(ctx, ses)
	if h.useUnreliable {
		go h.readUnreliable(ctx, ses)
	}

	<-ses.done
}

func (h *connService) read(ctx context.Context, ses *Session) {
	defer close(ses.done)

	for {
		msgs, err := h.codec.Decode(ses.peer)
		if err != nil {
			select {
			case <-ctx.Done():
				break
			default:
				// TODO Log
			}
		}

		for _, msg := range msgs {
			h.msgService.Handle(ctx, ses, msg)
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
				// TODO Log
			}
		}

		_ = data

		// TODO Handle unreliable
	}
}
