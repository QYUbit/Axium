package quichub

import (
	"github.com/quic-go/quic-go"
)

type Client struct {
	id   string
	conn quic.Connection
	send chan []byte
}

func NewClient(id string, conn quic.Connection) *Client {
	return &Client{
		id:   id,
		conn: conn,
		send: make(chan []byte, 256),
	}
}

func (c *Client) ReadPump(hub *Hub) {
	defer func() {
		hub.UnregisterClient(c.id)
	}()

	for {
		stream, err := c.conn.AcceptStream(hub.ctx)
		if err != nil {
			break
		}

		buf := bufferPool.Get().([]byte)
		defer bufferPool.Put(buf)

		n, err := stream.Read(buf)
		if err != nil {
			break
		}
		hub.onMessage(c.id, buf[:n])
		bufferPool.Put(buf)
	}
}

func (c *Client) WritePump(hub *Hub) {
	for {
		select {
		case <-hub.ctx.Done():
			return

		case message := <-c.send:
			stream, err := c.conn.OpenStreamSync(hub.ctx)
			if err != nil {
				break
			}
			stream.Write(message)
		}
	}
}
