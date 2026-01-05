package quic

import (
	"context"
	"io"

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

func (p *peer) OpenStream(ctx context.Context) (io.ReadWriteCloser, error) {
	stream, err := p.conn.OpenStreamSync(ctx)
	return stream, err
}

func (p *peer) AcceptStream(ctx context.Context) (io.ReadWriteCloser, error) {
	stream, err := p.conn.AcceptStream(ctx)
	return stream, err
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

func (t *QuicTransport) Listen(ctx context.Context) {

}

func (t *QuicTransport) Accept(ctx context.Context) (*peer, error) {
	conn, err := t.listener.Accept(ctx)
	if err != nil {
		return nil, err
	}
	return newPeer(conn), nil
}

func (t *QuicTransport) Close() error {
	return t.listener.Close()
}
