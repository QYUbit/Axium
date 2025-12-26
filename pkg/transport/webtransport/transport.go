// Package webtransport implements transport.Transport using WebTransport protocol.
package webtransport

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/QYUbit/Axium/pkg/transport"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
)

var bufferPool = &sync.Pool{
	New: func() any {
		buf := make([]byte, 1024)
		return &buf
	},
}

// WebTransport implements transport.Transport using WebTransport protocol
type WebTransport struct {
	address string
	server  *webtransport.Server

	clients  map[string]*client
	clientMu sync.RWMutex

	operations chan hubOperation

	connectChan    chan transport.Connection
	disconnectChan chan string
	messageChan    chan transport.Message
	errorChan      chan error

	idGenerator func() string

	closed          atomic.Bool
	closeOnce       sync.Once
	cancel          context.CancelFunc
	connectionsDone chan struct{}
	operationDone   chan struct{}
}

// NewWebTransport creates a new WebTransport instance
func NewWebTransport(server *webtransport.Server) *WebTransport {
	t := &WebTransport{
		server:         server,
		clients:        make(map[string]*client),
		operations:     make(chan hubOperation, 100),
		connectChan:    make(chan transport.Connection, 10),
		disconnectChan: make(chan string, 10),
		messageChan:    make(chan transport.Message, 100),
		errorChan:      make(chan error, 5),
	}

	return t
}

func (t *WebTransport) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	t.cancel = cancel

	t.connectionsDone = make(chan struct{})
	t.operationDone = make(chan struct{})

	go t.run(ctx)

	go func() {
		if err := t.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.transmitError(fmt.Errorf("server error: %v", err))
		}
		close(t.connectionsDone)
	}()

	return nil
}

func getAddress(r *http.Request) string {
	// Use X-Forwarded-For ?

	xRealIP := r.Header.Get("X-Real-IP")
	if xRealIP != "" {
		if ip := net.ParseIP(xRealIP); ip != nil {
			return xRealIP
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (t *WebTransport) run(ctx context.Context) {
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

func (t *WebTransport) handleOperation(op hubOperation, ctx context.Context) {
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
			client.session.CloseWithError(webtransport.SessionErrorCode(op.Code), string(op.Message))
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
			case <-ctx.Done():
				err = ctx.Err()
			}
		}
	}

	if op.Response != nil {
		op.Response <- err
	}
}

func (t *WebTransport) transmitError(err error) {
	select {
	case t.errorChan <- err:
	case <-time.After(time.Second):
		return
	}
}

func (t *WebTransport) transmitMessage(id string, data []byte) {
	select {
	case t.messageChan <- transport.Message{ClientId: id, Data: data}:
	case <-time.After(time.Second):
		return
	}
}

func (t *WebTransport) isClosed() bool {
	return t.closed.Load()
}

func (t *WebTransport) Close() error {
	t.closed.Store(true)

	var lastError error

	t.closeOnce.Do(func() {
		if t.cancel != nil {
			t.cancel()
		}

		// Close all client connections
		t.clientMu.Lock()
		for _, client := range t.clients {
			close(client.send)
			client.session.CloseWithError(0, "server closing")
		}
		t.clientMu.Unlock()

		<-t.connectionsDone
		<-t.operationDone

		close(t.connectChan)
		close(t.disconnectChan)
		close(t.messageChan)
		close(t.errorChan)

		if t.server != nil {
			lastError = t.server.Close()
		}
	})

	return lastError
}

func (t *WebTransport) HandleUpgrade(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	session, err := t.server.Upgrade(w, r)
	if err != nil {
		t.transmitError(fmt.Errorf("failed upgrading connection: %v", err))
		return
	}

	var id string
	if t.idGenerator != nil {
		id = t.idGenerator()
	} else {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			id = fmt.Sprintf("%d", time.Now().UnixNano())
		} else {
			id = hex.EncodeToString(b)
		}
	}

	if err := t.registerClient(id, session); err != nil {
		t.transmitError(err)
		return
	}

	select {
	case t.connectChan <- transport.Connection{
		ClientId:   id,
		RemoteAddr: getAddress(r),
	}:
	case <-time.After(time.Second):
	case <-ctx.Done():
	}
}

func (t *WebTransport) registerClient(clientId string, session *webtransport.Session) error {
	if t.isClosed() {
		return ErrTransportClosed
	}

	client := newClient(clientId, session)
	response := make(chan error, 1)

	select {
	case t.operations <- hubOperation{
		Type:     opRegisterClient,
		Client:   client,
		Response: response,
	}:
	case <-time.After(time.Second):
		return errors.New("timeout registering client")
	}

	return <-response
}

func (t *WebTransport) CloseClient(id string, code int, reason string) error {
	if t.isClosed() {
		return ErrTransportClosed
	}

	response := make(chan error, 1)

	select {
	case t.operations <- hubOperation{
		Type:     opUnregisterClient,
		ClientId: id,
		Code:     code,
		Message:  []byte(reason),
		Response: response,
	}:
	case <-time.After(time.Second):
		return errors.New("timeout closing client")
	}

	return <-response
}

func (t *WebTransport) GetClients() []string {
	t.clientMu.RLock()
	defer t.clientMu.RUnlock()

	ids := make([]string, 0, len(t.clients))
	for id := range t.clients {
		ids = append(ids, id)
	}
	return ids
}

func (t *WebTransport) Send(clientId string, message []byte, reliable bool) error {
	if t.isClosed() {
		return ErrTransportClosed
	}

	response := make(chan error, 1)

	select {
	case t.operations <- hubOperation{
		Type:     opSendMessage,
		ClientId: clientId,
		Message:  message,
		Response: response,
		Reliable: reliable,
	}:
	case <-time.After(time.Second):
		return errors.New("timeout sending message")
	}

	return <-response
}

func (t *WebTransport) Broadcast(clientIds []string, message []byte, reliable bool) error {
	if len(clientIds) == 0 {
		return nil
	}

	errChan := make(chan error, len(clientIds))
	for _, id := range clientIds {
		select {
		case t.operations <- hubOperation{
			Type:     opSendMessage,
			ClientId: id,
			Message:  message,
			Response: errChan,
			Reliable: reliable,
		}:
		case <-time.After(time.Second):
			errChan <- fmt.Errorf("timeout broadcasting to client %s", id)
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

func (t *WebTransport) Connections() <-chan transport.Connection {
	return t.connectChan
}

func (t *WebTransport) Disconnections() <-chan string {
	return t.disconnectChan
}

func (t *WebTransport) Messages() <-chan transport.Message {
	return t.messageChan
}

func (t *WebTransport) Errors() <-chan error {
	return t.errorChan
}

func (t *WebTransport) SetIdGenerator(idGenerator func() string) {
	t.idGenerator = idGenerator
}

func Sample() {
	ctx := context.Background()

	webtransportServer := &webtransport.Server{
		H3: http3.Server{
			Addr: "0.0.0.0:8080",
		},
	}

	transport := NewWebTransport(webtransportServer)

	http.HandleFunc("/webtransport", func(w http.ResponseWriter, r *http.Request) {
		transport.HandleUpgrade(ctx, w, r)
	})

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		transport.Start(ctx)
	}()

	<-ctx.Done()

	transport.Close()
}
