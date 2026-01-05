package quic

import (
	"context"

	t1 "github.com/QYUbit/Axium/pkg/transport_v1"
	quicgo "github.com/quic-go/quic-go"
)

type peer struct {
	conn *quicgo.Conn
}

func newPeer(conn *quicgo.Conn) *peer {
	return &peer{
		conn: conn,
	}
}

func (p *peer) Send(b []byte) error {
	return p.conn.SendDatagram(b)
}

func (p *peer) Receive() ([]byte, error) {
	return p.conn.ReceiveDatagram(context.TODO())
}

func (p *peer) RemoteAddr() string {
	return p.RemoteAddr()
}

func (p *peer) Close() error {
	return p.conn.CloseWithError(quicgo.ApplicationErrorCode(0), "")
}

type QuicTransport struct {
	listener *quicgo.Listener
}

func NewQuicTransport(listener *quicgo.Listener) *QuicTransport {
	return &QuicTransport{
		listener: listener,
	}
}

func (t *QuicTransport) Listen(ctx context.Context) error {
	l, err := quicgo.ListenAddr("", nil, nil)
	if err != nil {
		return err
	}
	t.listener = l
}

func (t *QuicTransport) Accept(ctx context.Context) (t1.Connection, error) {
	conn, err := t.listener.Accept(ctx)
	if err != nil {
		return nil, err
	}
	return newPeer(conn), nil
}

func (t *QuicTransport) Close() error {
	return t.listener.Close()
}
