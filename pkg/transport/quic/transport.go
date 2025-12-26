// Package quic implements transport.Transport using quic-go.
package quic

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QYUbit/Axium/pkg/transport"
	"github.com/quic-go/quic-go"
)

// Implements transport.Transport
type QuicTransport struct {
	address    string
	tlsConfig  *tls.Config
	quicConfig *quic.Config

	listener *quic.Listener

	clients  map[string]*client
	clientMu sync.RWMutex

	operations chan hubOperation

	connectChan    chan transport.Connection
	disconnectChan chan string
	messageChan    chan transport.Message
	errorChan      chan error

	idGenerator func() string
	validator   func(remoteAddr string) (bool, string)

	closed          atomic.Bool
	closeOnce       sync.Once
	cancel          context.CancelFunc
	connectionsDone chan struct{}
	operationDone   chan struct{}
}

func NewQuicTransport(address string, tlsConf *tls.Config, config *quic.Config) *QuicTransport {
	t := &QuicTransport{
		address:        address,
		tlsConfig:      tlsConf,
		quicConfig:     config,
		clients:        make(map[string]*client),
		operations:     make(chan hubOperation, 100),
		connectChan:    make(chan transport.Connection, 10),
		disconnectChan: make(chan string, 10),
		messageChan:    make(chan transport.Message, 100),
		errorChan:      make(chan error, 5),
	}

	return t
}

func (t *QuicTransport) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	t.cancel = cancel

	t.connectionsDone = make(chan struct{})
	t.operationDone = make(chan struct{})

	go t.run(ctx)

	listener, err := quic.ListenAddr(t.address, t.tlsConfig, t.quicConfig)
	if err != nil {
		return err
	}
	t.listener = listener

	go t.acceptConnections(ctx)
	return nil
}

func (t *QuicTransport) acceptConnections(ctx context.Context) {
	defer close(t.connectionsDone)

	for {
		conn, err := t.listener.Accept(ctx)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				t.transmitError(fmt.Errorf("failed accepting connection: %s", err))
				continue
			}
		}

		if t.validator != nil {
			accept, reason := t.validator(conn.RemoteAddr().String())
			if !accept {
				conn.CloseWithError(quic.ApplicationErrorCode(0x000a), reason)
				return
			}
		}

		var id string
		if t.idGenerator != nil {
			id = t.idGenerator()
		} else {
			b := make([]byte, 16)
			if _, err := rand.Read(b); err != nil {
				id = fmt.Sprintf("%d", time.Now().UnixNano())
			}
			id = hex.EncodeToString(b)
		}

		if err := t.registerClient(id, conn); err != nil {
			t.transmitError(err)
		}

		select {
		case t.connectChan <- transport.Connection{}:
		case <-time.After(time.Second):
			continue
		}
	}
}

func (t *QuicTransport) run(ctx context.Context) {
	defer func() {
		close(t.operationDone)
		close(t.operations)
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case op := <-t.operations:
			t.handleOperation(op, ctx)
		}
	}
}

func (t *QuicTransport) handleOperation(op hubOperation, ctx context.Context) {
	var err error

	switch op.Type {
	case opRegisterClient:
		t.clientMu.Lock()
		t.clients[op.Client.id] = op.Client
		t.clientMu.Unlock()
		go op.Client.readPump(t, ctx)
		go op.Client.datagramPump(t, ctx)
		go op.Client.writePump(t, ctx)

	case opUnregisterClient:
		t.clientMu.Lock()
		if client, ok := t.clients[op.ClientId]; ok {
			delete(t.clients, op.ClientId)
			close(client.send)
			client.conn.CloseWithError(quic.ApplicationErrorCode(op.Code), string(op.Message))
		}
		t.clientMu.Unlock()

	case opSendMessage:
		t.clientMu.RLock()
		client, exists := t.clients[op.ClientId]
		t.clientMu.RUnlock()

		if !exists {
			err = ErrClientNotFound{op.ClientId}
		} else {
			select {
			case client.send <- outgoingMessage{Content: op.Message, Reliable: op.Reliable}:
				// Message sent successfully
			case <-time.After(time.Second):
				err = fmt.Errorf("timeout sending message to client %s", op.ClientId)
			}
		}
	}

	if op.Response != nil {
		op.Response <- err
	}
}

func (t *QuicTransport) transmitError(err error) {
	select {
	case t.errorChan <- err:
	case <-time.After(time.Second):
		// Consumer timeout
		return
	}
}

func (t *QuicTransport) transmitMessage(id string, data []byte) {
	select {
	case t.messageChan <- transport.Message{ClientId: id, Data: data}:
	case <-time.After(time.Second):
		return
	}
}

func (t *QuicTransport) isClosed() bool {
	return t.closed.Load()
}

func (t *QuicTransport) Close() error {
	t.closed.Store(true)

	var lastError error

	t.closeOnce.Do(func() {
		t.cancel()

		<-t.connectionsDone
		<-t.operationDone

		close(t.connectChan)
		close(t.disconnectChan)
		close(t.messageChan)
		close(t.errorChan)

		lastError = t.listener.Close()
	})

	return lastError
}

func (t *QuicTransport) registerClient(clientId string, conn *quic.Conn) error {
	if t.isClosed() {
		return ErrTransportClosed
	}

	client := newClient(clientId, conn)
	response := make(chan error, 1)

	t.operations <- hubOperation{
		Type:     opRegisterClient,
		Client:   client,
		Response: response,
	}

	return <-response
}

func (t *QuicTransport) CloseClient(id string, code int, reason string) error {
	if t.isClosed() {
		return ErrTransportClosed
	}

	response := make(chan error, 1)

	t.operations <- hubOperation{
		Type:     opUnregisterClient,
		ClientId: id,
		Code:     code,
		Message:  []byte(reason),
		Response: response,
	}

	return <-response
}

func (t *QuicTransport) GetClients() []string {
	t.clientMu.RLock()
	defer t.clientMu.RUnlock()

	ids := make([]string, 0, len(t.clients))
	for id := range t.clients {
		ids = append(ids, id)
	}
	return ids
}

func (t *QuicTransport) Send(clientId string, message []byte, reliable bool) error {
	if t.isClosed() {
		return ErrTransportClosed
	}

	response := make(chan error, 1)

	t.operations <- hubOperation{
		Type:     opSendMessage,
		ClientId: clientId,
		Message:  message,
		Response: response,
		Reliable: reliable,
	}

	return <-response
}

func (t *QuicTransport) Broadcast(clientIds []string, message []byte, reliable bool) error {
	if len(clientIds) == 0 {
		return nil
	}

	errChan := make(chan error, len(clientIds))
	for _, id := range clientIds {
		t.operations <- hubOperation{
			Type:     opSendMessage,
			ClientId: id,
			Message:  message,
			Response: errChan,
			Reliable: reliable,
		}
	}

	var errs []error
	for range clientIds {
		if err := <-errChan; err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("broadcast failed for %d/%d clients: %v", len(errs), len(clientIds), errs[0])
	}
	return nil
}

func (t *QuicTransport) Connections() <-chan transport.Connection {
	return t.connectChan
}

func (t *QuicTransport) Disconnections() <-chan string {
	return t.disconnectChan
}

func (t *QuicTransport) Messages() <-chan transport.Message {
	return t.messageChan
}

func (t *QuicTransport) Errors() <-chan error {
	return t.errorChan
}

func (t *QuicTransport) SetIdGenerator(idGenerator func() string) {
	t.idGenerator = idGenerator
}

func (t *QuicTransport) SetValidator(validator func(string) (bool, string)) {
	t.validator = validator
}
