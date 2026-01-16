package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"time"

	"github.com/QYUbit/Axium/pkg/transport"
	"github.com/quic-go/quic-go"
)

var ErrTransportNotInitialized = errors.New("quic transport has not been initialized")

var closeCodeMap = map[transport.CloseCode]quic.ApplicationErrorCode{}

type emptyAddr struct{}

func (emptyAddr) Network() string { return "none" }
func (emptyAddr) String() string  { return "uninitialized" }

type Transport struct {
	address  string
	tlsCfg   *tls.Config
	quicCfg  *quic.Config
	listener *quic.Listener
}

func NewTransport(addr string, tlsCfg *tls.Config, quicCfg *quic.Config) *Transport {
	return &Transport{
		address: addr,
		tlsCfg:  tlsCfg,
		quicCfg: quicCfg,
	}
}

func (t *Transport) Listen() error {
	l, err := quic.ListenAddr(t.address, t.tlsCfg, t.quicCfg)
	if err != nil {
		return err
	}
	t.listener = l
	return nil
}

func (t *Transport) Accept(ctx context.Context) (transport.Peer, error) {
	if t.listener == nil {
		return nil, ErrTransportNotInitialized
	}

	conn, err := t.listener.Accept(ctx)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}

	return &Peer{
		conn:          conn,
		controlStream: stream,
	}, nil
}

func (t *Transport) Close() error {
	if t.listener == nil {
		return ErrTransportNotInitialized
	}
	return t.listener.Close()
}

func (t *Transport) Addr() net.Addr {
	if t.listener == nil {
		return emptyAddr{}
	}
	return t.listener.Addr()
}

type Peer struct {
	conn          *quic.Conn
	controlStream *quic.Stream
}

func (p *Peer) Read(b []byte) (int, error) {
	return p.controlStream.Read(b)
}

func (p *Peer) Write(b []byte) (int, error) {
	return p.controlStream.Write(b)
}

func (p *Peer) SendUnreliable(b []byte) error {
	return p.conn.SendDatagram(b)
}

func (p *Peer) ReceiveUnreliable() ([]byte, error) {
	return p.conn.ReceiveDatagram(p.conn.Context())
}

func (p *Peer) Close(code transport.CloseCode, reason string) error {
	appCode, ok := closeCodeMap[code]
	if !ok {
		appCode = 0x0
	}
	return p.conn.CloseWithError(appCode, reason)
}

func (p *Peer) LocalAddr() net.Addr {
	return p.conn.LocalAddr()
}

func (p *Peer) RemoteAddr() net.Addr {
	return p.conn.RemoteAddr()
}
