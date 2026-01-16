package websockets

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/QYUbit/Axium/pkg/transport"
	"github.com/gorilla/websocket"
)

var closeCodeMap = map[transport.CloseCode]int{}

type Transport struct {
	upgrader    *websocket.Upgrader
	connections chan *websocket.Conn
}

func (t *Transport) Listen() error {
	return nil
}

func (t *Transport) Upgrade(w http.ResponseWriter, r *http.Request, responseHeader http.Header) error {
	conn, err := t.upgrader.Upgrade(w, r, responseHeader)
	if err != nil {
		return err
	}
	t.connections <- conn
	return nil
}

func (t *Transport) Accept(ctx context.Context) (transport.Peer, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case conn := <-t.connections:
		return &Peer{conn: conn}, nil
	}
}

func (t *Transport) Close() error {
	return nil
}

func (t *Transport) Addr() net.Addr {
	// TODO
	return nil
}

type Peer struct {
	conn *websocket.Conn
}

func (p *Peer) Read(b []byte) (int, error) {
	_, pp, err := p.conn.ReadMessage()
	if err != nil {
		return 0, err
	}
	return copy(b, pp), nil
}

func (p *Peer) Write(b []byte) (int, error) {
	if err := p.conn.WriteMessage(websocket.BinaryMessage, b); err != nil {
		return 0, err
	}
	return len(b), nil
}

func (p *Peer) SendUnreliable(b []byte) error {
	return p.conn.WriteMessage(websocket.BinaryMessage, b)
}

func (p *Peer) ReceiveUnreliable() ([]byte, error) {
	_, b, err := p.conn.ReadMessage()
	return b, err
}

func (p *Peer) Close(code transport.CloseCode, reason string) error {
	wsCode, ok := closeCodeMap[code]
	if !ok {
		wsCode = 1000
	}

	var lastErr error

	err := p.conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(wsCode, reason),
		time.Now().Add(time.Second),
	)
	if err != nil {
		lastErr = err
	}

	if err := p.conn.Close(); err != nil {
		lastErr = err
	}
	return lastErr
}

func (p *Peer) LocalAddr() net.Addr {
	return p.conn.LocalAddr()
}

func (p *Peer) RemoteAddr() net.Addr {
	return p.conn.RemoteAddr()
}
