package webtransport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/quic-go/webtransport-go"
)

type client struct {
	id      string
	session *webtransport.Session
	send    chan outgoingMessage
	closed  atomic.Bool
}

func newClient(id string, session *webtransport.Session) *client {
	return &client{
		id:      id,
		session: session,
		send:    make(chan outgoingMessage, 256),
	}
}

func (c *client) close(t *WebTransport) {
	if c.closed.CompareAndSwap(false, true) {
		t.CloseClient(c.id, 0, "client closed")

		select {
		case t.disconnectChan <- c.id:
		case <-time.After(time.Second):
			return
		}
	}
}

func (c *client) readPump(t *WebTransport, ctx context.Context) {
	defer c.close(t)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		stream, err := c.session.AcceptStream(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			break
		}

		go c.handleStream(t, stream)
	}
}

func (c *client) handleStream(t *WebTransport, stream *webtransport.Stream) {
	defer stream.Close()

	buf := *bufferPool.Get().(*[]byte)
	defer bufferPool.Put(&buf)

	n, err := stream.Read(buf)
	if err != nil && err != io.EOF {
		return
	}

	if n > 0 {
		message := make([]byte, n)
		copy(message, buf[:n])
		go t.transmitMessage(c.id, message)
	}
}

func (c *client) datagramPump(t *WebTransport, ctx context.Context) {
	defer c.close(t)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		message, err := c.session.ReceiveDatagram(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			break
		}

		go t.transmitMessage(c.id, message)
	}
}

func (c *client) writePump(t *WebTransport, ctx context.Context) {
	defer func() {
		if err := recover(); err != nil {
			t.transmitError(fmt.Errorf("writePump panic for client %s: %v", c.id, err))
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case message, ok := <-c.send:
			if !ok {
				return
			}

			if !message.Reliable {
				if err := c.session.SendDatagram(message.Content); err != nil {
					t.transmitError(fmt.Errorf("failed sending datagram to client %s: %v", c.id, err))
				}
				continue
			}

			stream, err := c.session.OpenStreamSync(ctx)
			if err != nil {
				t.transmitError(fmt.Errorf("failed opening stream for client %s: %v", c.id, err))
				continue
			}

			_, err = stream.Write(message.Content)
			if err != nil {
				t.transmitError(fmt.Errorf("failed writing to stream for client %s: %v", c.id, err))
				stream.Close()
				continue
			}

			stream.Close()
		}
	}
}
