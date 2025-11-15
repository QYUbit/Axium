package quic

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
)

var bufferPool = &sync.Pool{
	New: func() any {
		buf := make([]byte, 1024)
		return &buf
	},
}

type client struct {
	id     string
	conn   quic.Connection
	send   chan outgoingMessage
	closed atomic.Bool
}

func newClient(id string, conn quic.Connection) *client {
	return &client{
		id:   id,
		conn: conn,
		send: make(chan outgoingMessage, 256),
	}
}

func (c *client) close(t *QuicTransport) {
	if c.closed.CompareAndSwap(false, true) {
		t.CloseClient(c.id, 0, "client closed")

		select {
		case t.disconnectChan <- c.id:
		case <-time.After(time.Second):
			return
		}
	}
}

func (c *client) readPump(t *QuicTransport, ctx context.Context) {
	defer c.close(t)

	for {
		stream, err := c.conn.AcceptStream(ctx)
		if err != nil {
			break
		}

		buf := *bufferPool.Get().(*[]byte)
		n, err := stream.Read(buf)
		stream.Close()
		if err != nil {
			bufferPool.Put(&buf)
			break
		}

		message := make([]byte, n)
		copy(message, buf[:n])
		bufferPool.Put(&buf)

		go t.transmitMessage(c.id, message)
	}
}

func (c *client) datagramPump(t *QuicTransport, ctx context.Context) {
	defer c.close(t)

	for {
		message, err := c.conn.ReceiveDatagram(ctx)
		if err != nil {
			break
		}

		go t.transmitMessage(c.id, message)
	}
}

func (c *client) writePump(t *QuicTransport, ctx context.Context) {
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
				if err := c.conn.SendDatagram(message.Content); err != nil {
					t.transmitError(fmt.Errorf("failed sending datagram to client %s: %v", c.id, err))
				}
				continue
			}

			stream, err := c.conn.OpenStreamSync(ctx)
			if err != nil {
				t.transmitError(fmt.Errorf("failed opening stream for client %s: %v", c.id, err))
				continue
			}

			_, err = stream.Write(message.Content)
			if err != nil {
				t.transmitError(fmt.Errorf("failed writing to stream for client %s: %v", c.id, err))
				continue
			}

			stream.Close()
		}
	}
}
