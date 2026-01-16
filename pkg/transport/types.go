// Package transport serves as an abstraction for bi-directional transports.
// It provides interfaces that can be implemented by concrete transport adapters.
package transport

import (
	"context"
	"net"
)

// CloseCode represents numerical error codes transmitted when
// closing a connection. These will be mapped by the adapter to
// protocol-specific codes.
type CloseCode int

// Peer represents an active connection to a remote participant.
type Peer interface {
	// Read reads reliable data from the connection / stream. The returned data can be a
	// single message as well as a continuous stream. If the network adapter
	// uses streams, callers may want to implement some sort of framing.
	Read(p []byte) (int, error)

	// Write writes reliable data to the connection / stream.
	// It guarantees delivery and preserves the order of the transmitted bytes.
	Write(p []byte) (int, error)

	// SendUnreliable sends an atomic message without delivery guarantees.
	// This is intended for high-frequency data where low latency is prioritized
	// over reliability (e.g. position updates).
	SendUnreliable(p []byte) error

	// ReceiveUnreliable waits for an incoming unreliable message. Unlike Read,
	// it is assured that this returns a discrete message, rather than a continuous stream.
	ReceiveUnreliable() ([]byte, error)

	// Close terminates the connection with the peer, sending a status code
	// and an optional reason for the closure.
	Close(code CloseCode, reason string) error

	// LocalAddr returns the local network address of the connection.
	LocalAddr() net.Addr

	// RemoteAddr returns the network address of the remote participant.
	RemoteAddr() net.Addr
}

// Transport defines the interface for bi-directional network implementations.
// A transport is responsible for accepting new connections (peers),
// regardless of whether it acts as a standalone server (UDP / QUIC)
// or as a bridge to an existing infrastructure (WebSockets / HTTP).
type Transport interface {
	// Listen initializes the transport and binds necessary resources.
	// The transport is only ready to accept connections after Listen
	// has been called.
	Listen() error

	// Accept waits for and returns the next incoming connection.
	// If the provided context is canceled, Accept stops waiting
	// and return the context error.
	Accept(ctx context.Context) (Peer, error)

	// Close closes the transport and releases all bound resources.
	Close() error

	// Addr returns the address the transport is listening on.
	// At the moment this may not return a real address.
	Addr() net.Addr
}
