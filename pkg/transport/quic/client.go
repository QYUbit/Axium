package quic

import (
	"fmt"
	"sync"
	"sync/atomic"

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
	send   chan message
	closed atomic.Bool
}

func newClient(id string, conn quic.Connection) *client {
	return &client{
		id:   id,
		conn: conn,
		send: make(chan message, 256),
	}
}

func (c *client) close(t *QuicTransport) {
	if c.closed.CompareAndSwap(false, true) {
		go t.onDisconnect(c.id)
		t.CloseClient(c.id, 0, "client closed")
	}
}

func (c *client) readPump(t *QuicTransport) {
	defer c.close(t)

	for {
		stream, err := c.conn.AcceptStream(t.ctx)
		if err != nil {
			break
		}

		buf := *bufferPool.Get().(*[]byte)
		n, err := stream.Read(buf)
		if err != nil {
			bufferPool.Put(&buf)
			break
		}

		message := make([]byte, n)
		copy(message, buf[:n])
		bufferPool.Put(&buf)

		go t.onMessage(c.id, message)
	}
}

func (c *client) datagramPump(t *QuicTransport) {
	defer c.close(t)

	for {
		message, err := c.conn.ReceiveDatagram(t.ctx)
		if err != nil {
			break
		}

		go t.onMessage(c.id, message)
	}
}

func (c *client) writePump(t *QuicTransport) {
	defer func() {
		if err := recover(); err != nil {
			t.onError(fmt.Errorf("writePump panic for client %s: %v", c.id, err))
		}
	}()

	for {
		select {
		case <-t.ctx.Done():
			return

		case message, ok := <-c.send:
			if !ok {
				return
			}

			if !message.Reliable {
				if err := c.conn.SendDatagram(message.Content); err != nil {
					t.onError(fmt.Errorf("failed sending datagram to client %s: %v", c.id, err))
				}
				return
			}

			stream, err := c.conn.OpenStreamSync(t.ctx)
			if err != nil {
				t.onError(fmt.Errorf("failed opening stream for client %s: %v", c.id, err))
				return
			}

			_, err = stream.Write(message.Content)
			if err != nil {
				t.onError(fmt.Errorf("failed writing to stream for client %s: %v", c.id, err))
				return
			}

			stream.Close()
		}
	}
}
