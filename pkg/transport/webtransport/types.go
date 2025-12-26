package webtransport

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
