package quic

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/quic-go/quic-go"
)

// Implements Transport
type QuicTransport struct {
	address      string
	tlsConfig    *tls.Config
	quicConfig   *quic.Config
	listener     *quic.Listener
	clients      map[string]*client
	operations   chan hubOperation
	ctx          context.Context
	cancel       context.CancelFunc
	clientMu     sync.RWMutex
	onConnect    func(string, func(string), func(string))
	onDisconnect func(string)
	onMessage    func(string, []byte)
	onError      func(error)
	closed       atomic.Bool
}

func NewQuicTransport(address string, tlsConf *tls.Config, config *quic.Config) *QuicTransport {
	t := &QuicTransport{
		address:    address,
		tlsConfig:  tlsConf,
		quicConfig: config,
		clients:    make(map[string]*client),
		operations: make(chan hubOperation, 100),
	}

	t.onConnect = func(_ string, accept func(string), reject func(string)) {
		fmt.Println("New client connected")
		accept(uuid.New().String())
	}

	t.onDisconnect = func(id string) {
		fmt.Printf("Client %s disconnected\n", id)
	}

	t.onMessage = func(id string, b []byte) {
		fmt.Printf("Received message by client %s: %s\n", id, b)
	}

	t.onError = func(err error) {
		fmt.Println("An unhandled error occurred:", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.ctx = ctx
	t.cancel = cancel

	return t
}

func (t *QuicTransport) Start() error {
	go t.run()

	listener, err := quic.ListenAddr(t.address, t.tlsConfig, t.quicConfig)
	if err != nil {
		return err
	}
	t.listener = listener

	go t.acceptConnections()
	return nil
}

func (t *QuicTransport) acceptConnections() {
	for {
		conn, err := t.listener.Accept(t.ctx)
		if err != nil {
			select {
			case <-t.ctx.Done():
				return
			default:
				t.onError(fmt.Errorf("failed accepting connection: %s", err))
				continue
			}
		}

		accept := func(id string) { t.registerClient(id, conn) }

		reject := func(reason string) {
			if reason == "" {
				reason = "unauthorized"
			}
			conn.CloseWithError(quic.ApplicationErrorCode(0x000a), reason)
		}

		go t.onConnect(conn.RemoteAddr().String(), accept, reject)
	}
}

func (t *QuicTransport) run() {
	for {
		select {
		case <-t.ctx.Done():
			return

		case op := <-t.operations:
			t.handleOperation(op)
		}
	}
}

func (t *QuicTransport) handleOperation(op hubOperation) {
	var err error

	switch op.Type {
	case opRegisterClient:
		t.clientMu.Lock()
		t.clients[op.Client.id] = op.Client
		t.clientMu.Unlock()
		go op.Client.readPump(t)
		go op.Client.datagramPump(t)
		go op.Client.writePump(t)

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
			case client.send <- message{Content: op.Message, Reliable: op.Reliable}:
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

func (t *QuicTransport) isClosed() bool {
	return t.closed.Load()
}

func (t *QuicTransport) Close() error {
	t.closed.Store(true)
	t.cancel()
	return t.listener.Close()
}

func (t *QuicTransport) registerClient(clientId string, conn quic.Connection) error {
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
	if t.isClosed() {
		return ErrTransportClosed
	}

	response := make(chan error)

	for _, id := range clientIds {
		t.operations <- hubOperation{
			Type:     opSendMessage,
			ClientId: id,
			Message:  message,
			Response: response,
			Reliable: reliable,
		}
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

func (t *QuicTransport) OnConnect(fn func(string, func(string), func(string))) {
	t.onConnect = fn
}

func (t *QuicTransport) OnDisconnect(fn func(string)) {
	t.onDisconnect = fn
}

func (t *QuicTransport) OnMessage(fn func(string, []byte)) {
	t.onMessage = fn
}

func (t *QuicTransport) OnError(fn func(error)) {
	t.onError = fn
}
