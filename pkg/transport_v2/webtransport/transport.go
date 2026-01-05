package webtransport

import (
	"context"
	"net/http"

	t2 "github.com/QYUbit/Axium/pkg/transport_v2"
	"github.com/quic-go/webtransport-go"
)

type stream struct {
	str *webtransport.Stream
}

func (s *stream) Read(p []byte) (int, error) {
	return s.str.Read(p)
}

func (s *stream) Write(p []byte) (int, error) {
	return s.str.Write(p)
}

func (s *stream) Close() error {
	return s.str.Close()
}

func (s *stream) ID() int64 {
	return int64(s.str.StreamID())
}

type peer struct {
	ses *webtransport.Session
}

func (p *peer) LocalAddr() string {
	return p.LocalAddr()
}

func (p *peer) RemoteAddr() string {
	return p.RemoteAddr()
}

func (p *peer) Close() error {
	return p.ses.CloseWithError(webtransport.SessionErrorCode(0), "") // TODO
}

func (p *peer) OpenStream(ctx context.Context) (t2.Stream, error) {
	str, err := p.ses.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return &stream{str: str}, nil
}

func (p *peer) AcceptStream(ctx context.Context) (t2.Stream, error) {
	str, err := p.ses.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}
	return &stream{str: str}, nil
}

func (p *peer) Send(b []byte) error {
	return p.ses.SendDatagram(b)
}

func (p *peer) Receive() ([]byte, error) {
	return p.ses.ReceiveDatagram(context.TODO())
}

type WebTransport struct {
	server    *webtransport.Server
	connQueue chan *peer
	cancel    context.CancelFunc
	closed    chan struct{}
}

func NewWebTransport(sever *webtransport.Server) *WebTransport {
	return &WebTransport{
		server:    sever,
		connQueue: make(chan *peer),
		closed:    make(chan struct{}),
	}
}

func (t *WebTransport) Listen() error {
	return t.server.ListenAndServe()
}

func (t *WebTransport) Accept(ctx context.Context) (t2.Connection, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case p := <-t.connQueue:
		return p, nil
	}
}

func (t *WebTransport) Close() error {
	if t.cancel != nil {
		t.cancel()
	}
	<-t.closed
	return t.server.Close()
}

func (t *WebTransport) HandleUpgrade(w http.ResponseWriter, r *http.Request) error {
	ses, err := t.server.Upgrade(w, r)
	if err != nil {
		return err
	}
	p := &peer{ses: ses}
	t.connQueue <- p // Timeout ?
	return nil
}
