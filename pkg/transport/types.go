// Package transport serves as a network abstraction at (not necessarily) transport level.
package transport

import "context"

type Message struct {
	ClientId string
	Data     []byte
}

type Connection struct {
	ClientId   string
	RemoteAddr string
}

type IdGenerator func() string

type ConnectionValidator func(remoteAddr string) (accept bool, reason string)

type Transport interface {
	Start(ctx context.Context) error
	Close() error
	Send(clientId string, data []byte, reliable bool) error
	Broadcast(clientId []string, data []byte, reliable bool) error
	CloseClient(clientId string, code int, reason string) error
	GetClients() (clientIds []string)
	Messages() <-chan Message
	Connections() <-chan Connection
	Disconnections() <-chan string
	Errors() <-chan error
	SetIdGenerator(idGenerator IdGenerator)
	SetValidator(validator ConnectionValidator)
}
