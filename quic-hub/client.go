package quichub

import (
	"fmt"

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
		n, err := stream.Read(buf)
		if err != nil {
			bufferPool.Put(buf)
			break
		}

		message := make([]byte, n)
		copy(message, buf[:n])
		bufferPool.Put(buf)

		hub.onMessage(c.id, message)
	}
}

func (c *Client) WritePump(hub *Hub) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Printf("WritePump panic for client %s: %v\n", c.id, err)
		}
	}()

	for {
		select {
		case <-hub.ctx.Done():
			return

		case message, ok := <-c.send:
			if !ok {
				return
			}

			stream, err := c.conn.OpenStreamSync(hub.ctx)
			if err != nil {
				fmt.Printf("Error opening stream for client %s: %v\n", c.id, err)
				return
			}

			_, err = stream.Write(message)
			if err != nil {
				fmt.Printf("Error writing to stream for client %s: %v\n", c.id, err)
				return
			}

			stream.Close()
		}
	}
}
