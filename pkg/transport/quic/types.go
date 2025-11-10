package quic

import (
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

type message struct {
	Content  []byte
	Reliable bool
}

type Transport interface {
	Send(to string, data []byte, relable bool) error
	Broadcast(to []string, data []byte, reliable bool) error
	CloseClient(client string, code int, reason string) error
	OnConnect(func(remoteAddr string, accept func(id string), reject func(reason string)))
	OnDisconnect(func(client string))
	OnMessage(func(from string, data []byte))
	OnError(func(err error))
	GetClients() []string
	Start() error
	Close() error
}
