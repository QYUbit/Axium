package quichub

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QYUbit/Axium/axium"
	"github.com/google/uuid"
	"github.com/quic-go/quic-go"
)

var (
	ErrClientNotFound  = errors.New("client not found")
	ErrTopicNotFound   = errors.New("topic not found")
	ErrTransportClosed = errors.New("transport is closed")
)

var bufferPool = &sync.Pool{
	New: func() any {
		return make([]byte, 1024)
	},
}

// Implements axium.AxiumConnection
type connectionWrapper struct {
	conn quic.Connection
}

func (cw connectionWrapper) Close(code int, errorString string) {
	cw.conn.CloseWithError(quic.ApplicationErrorCode(code), errorString)
}

func (cw connectionWrapper) GetRemoteAddress() string {
	return cw.conn.RemoteAddr().String()
}

type hubOperationType int

const (
	opRegisterClient hubOperationType = iota
	opUnregisterClient
	opSendMessage
	opCreateTopic
	opDeleteTopic
	opSubscribe
	opUnsubscribe
	opPublish
)

type message struct {
	Content  []byte
	Reliable bool
}

type hubOperation struct {
	Type     hubOperationType
	ClientId string
	TopicId  string
	Client   *client
	Topic    *topic
	Message  []byte
	Reliable bool
	Code     int
	Response chan error
}

// Implements axium.AxiumTransport
type QuicTransport struct {
	listener     *quic.Listener
	clients      map[string]*client
	topics       map[string]*topic
	operations   chan hubOperation
	ctx          context.Context
	cancel       context.CancelFunc
	clientMu     sync.RWMutex
	topicMu      sync.RWMutex
	onConnect    func(axium.AxiumConnection, func(string), func(string))
	onDisconnect func(string)
	onMessage    func(string, []byte)
	closed       atomic.Bool
}

func NewQuicTransport(address string, tlsConf *tls.Config, config *quic.Config) (*QuicTransport, error) {
	t := &QuicTransport{
		clients:    make(map[string]*client),
		topics:     make(map[string]*topic),
		operations: make(chan hubOperation, 100),
	}

	t.onConnect = func(_ axium.AxiumConnection, accept func(string), reject func(string)) {
		fmt.Println("New client connected")
		accept(uuid.New().String())
	}

	t.onDisconnect = func(id string) {
		fmt.Printf("Client %s disconnected\n", id)
	}

	t.onMessage = func(id string, b []byte) {
		fmt.Printf("Received message by client %s: %s\n", id, b)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.ctx = ctx
	t.cancel = cancel

	go t.run()

	listener, err := quic.ListenAddr(address, tlsConf, config)
	if err != nil {
		return nil, err
	}
	t.listener = listener

	go t.acceptConnections()

	return t, nil
}

func (t *QuicTransport) acceptConnections() {
	for {
		conn, err := t.listener.Accept(t.ctx)
		if err != nil {
			select {
			case <-t.ctx.Done():
				return
			default:
				fmt.Printf("failed accepting connection: %s\n", err)
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

		connWrapper := connectionWrapper{conn: conn}
		go t.onConnect(connWrapper, accept, reject)
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
			err = ErrClientNotFound
		} else {
			select {
			case client.send <- message{Content: op.Message, Reliable: op.Reliable}:
				// Message sent successfully
			case <-time.After(time.Second):
				err = fmt.Errorf("timeout sending message to client %s", op.ClientId)
			}
		}

	case opCreateTopic:
		t.topicMu.Lock()
		t.topics[op.Topic.id] = op.Topic
		t.topicMu.Unlock()
		go op.Topic.run(t.ctx)

	case opDeleteTopic:
		t.topicMu.Lock()
		if topic, ok := t.topics[op.TopicId]; ok {
			delete(t.topics, op.TopicId)
			close(topic.subscribe)
			close(topic.unsubscribe)
			close(topic.publish)
		}
		t.topicMu.Unlock()

	case opSubscribe:
		t.topicMu.RLock()
		topic, topicExists := t.topics[op.TopicId]
		t.topicMu.RUnlock()

		if !topicExists {
			err = ErrTopicNotFound
		} else {
			t.clientMu.RLock()
			client, clientExists := t.clients[op.ClientId]
			t.clientMu.RUnlock()

			if !clientExists {
				err = ErrClientNotFound
			} else {
				select {
				case topic.subscribe <- client:
					// Subscribed successfully
				case <-time.After(time.Second):
					err = fmt.Errorf("timeout subscribing client %s to topic %s", op.ClientId, op.TopicId)
				}
			}
		}

	case opUnsubscribe:
		t.topicMu.RLock()
		topic, topicExists := t.topics[op.TopicId]
		t.topicMu.RUnlock()

		if !topicExists {
			err = ErrTopicNotFound
		} else {
			t.clientMu.RLock()
			client, clientExists := t.clients[op.ClientId]
			t.clientMu.RUnlock()

			if !clientExists {
				err = ErrClientNotFound
			} else {
				select {
				case topic.unsubscribe <- client:
					// Unsubscribed successfully
				case <-time.After(time.Second):
					err = fmt.Errorf("timeout unsubscribing client %s from topic %s", op.ClientId, op.TopicId)
				}
			}
		}

	case opPublish:
		t.topicMu.RLock()
		topic, exists := t.topics[op.TopicId]
		t.topicMu.RUnlock()

		if !exists {
			err = ErrTopicNotFound
		} else {
			select {
			case topic.publish <- message{Content: op.Message, Reliable: op.Reliable}:
				// Published successfully
			case <-time.After(time.Second):
				err = fmt.Errorf("timeout publishing to topic %s", op.TopicId)
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

func (t *QuicTransport) CreateTopic(id string) error {
	if t.isClosed() {
		return ErrTransportClosed
	}

	topic := newTopic(id)
	response := make(chan error, 1)

	t.operations <- hubOperation{
		Type:     opCreateTopic,
		Topic:    topic,
		Response: response,
	}

	return <-response
}

func (t *QuicTransport) DeleteTopic(id string) error {
	if t.isClosed() {
		return ErrTransportClosed
	}

	response := make(chan error, 1)

	t.operations <- hubOperation{
		Type:     opDeleteTopic,
		TopicId:  id,
		Response: response,
	}

	return <-response
}

func (t *QuicTransport) Subscribe(clientId string, topicId string) error {
	if t.isClosed() {
		return ErrTransportClosed
	}

	response := make(chan error, 1)

	t.operations <- hubOperation{
		Type:     opSubscribe,
		ClientId: clientId,
		TopicId:  topicId,
		Response: response,
	}

	return <-response
}

func (t *QuicTransport) Unsubscribe(clientId string, topicId string) error {
	if t.isClosed() {
		return ErrTransportClosed
	}

	response := make(chan error, 1)

	t.operations <- hubOperation{
		Type:     opUnsubscribe,
		ClientId: clientId,
		TopicId:  topicId,
		Response: response,
	}

	return <-response
}

func (t *QuicTransport) Publish(topicId string, message []byte) error {
	if t.isClosed() {
		return ErrTransportClosed
	}

	response := make(chan error, 1)

	t.operations <- hubOperation{
		Type:     opPublish,
		TopicId:  topicId,
		Message:  message,
		Response: response,
	}

	return <-response
}

func (t *QuicTransport) GetClientIds() []string {
	t.clientMu.RLock()
	defer t.clientMu.RUnlock()

	ids := make([]string, 0, len(t.clients))
	for id := range t.clients {
		ids = append(ids, id)
	}
	return ids
}

func (t *QuicTransport) GetTopicIds() []string {
	t.topicMu.RLock()
	defer t.topicMu.RUnlock()

	ids := make([]string, 0, len(t.topics))
	for id := range t.topics {
		ids = append(ids, id)
	}
	return ids
}

func (t *QuicTransport) GetClientIdsOfTopic(topicId string) ([]string, error) {
	t.topicMu.RLock()
	defer t.topicMu.RUnlock()

	topic, exists := t.topics[topicId]
	if !exists {
		return nil, ErrTopicNotFound
	}
	return topic.getClientIds(), nil
}

func (t *QuicTransport) OnConnect(fn func(axium.AxiumConnection, func(string), func(string))) {
	t.onConnect = fn
}

func (t *QuicTransport) OnDisconnect(fn func(string)) {
	t.onDisconnect = fn
}

func (t *QuicTransport) OnMessage(fn func(string, []byte)) {
	t.onMessage = fn
}
