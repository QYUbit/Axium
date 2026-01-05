package t1

import "context"

type Reliability int

const (
	OnlyReliable   = Reliability(iota) // tcp, websockets
	OnlyUnreliable                     // udp
	SupportsBoth                       // quic, webtransport
)

type Capabilities struct {
	Reliability Reliability
}

type Connection interface {
	Send(p []byte) error
	Receive() ([]byte, error)
	LocalAddr() string
	RemoteAddr() string
	Close(code int, reason string) error
}

type Transport interface {
	Listen() error
	Accept(ctx context.Context) (Connection, error)
	Close() error
	Capabilities() Capabilities
}
