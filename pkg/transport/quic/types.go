package quic

import (
	"context"
	"errors"
	"fmt"
)

type ErrClientNotFound struct {
	ClientId string
}

func (e ErrClientNotFound) Error() string {
	return fmt.Sprintf("client %s not found", e.ClientId)
}

type ErrTopicNotFound struct {
	TopicId string
}

func (e ErrTopicNotFound) Error() string {
	return fmt.Sprintf("topic %s not found", e.TopicId)
}

var ErrTransportClosed = errors.New("transport is closed")

type hubOperationType int

const (
	opRegisterClient hubOperationType = iota
	opUnregisterClient
	opSendMessage
)

type hubOperation struct {
	Type     hubOperationType
	ClientId string
	Client   *client
	Message  []byte
	Reliable bool
	Code     int
	Response chan error
}

type outgoingMessage struct {
	Content  []byte
	Reliable bool
}

type Message struct {
	ClientId string
	Data     []byte
}

type ConnectionRequest struct {
	RemoteAddr string
}

type Transport interface {
	Start(ctx context.Context) error
	Close() error
	Send(clientId string, data []byte, reliable bool) error
	Broadcast(clientId []string, data []byte, reliable bool) error
	CloseClient(clientId string, code int, reason string) error
	GetClients() (clientIds []string)
	Messages() <-chan Message
	Connections() <-chan ConnectionRequest
	Disconnections() <-chan string
	Errors() <-chan error
}
