package t2

import (
	"context"
	"io"
)

type Reliability int

const (
	OnlyReliable   = Reliability(iota) // tcp, websockets
	OnlyUnreliable                     // udp
	SupportsBoth                       // quic, webtransport
)

type Capabilities struct {
	HasStreams  bool
	Reliability Reliability
}

type Stream interface {
	ID() int64
	io.ReadWriteCloser
}

type Connection interface {
	Send(p []byte) error
	Receive() ([]byte, error)
	OpenStream(ctx context.Context) (Stream, error)
	AcceptStream(ctx context.Context) (Stream, error)
	Close() error
	LocalAddr() string
	RemoteAddr() string
}

type Listener interface {
	Listen() error
	Accept(ctx context.Context) (Connection, error)
	Close() error
}
