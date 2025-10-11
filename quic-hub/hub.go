package quichub

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QYUbit/Axium/core"
	"github.com/google/uuid"
	"github.com/quic-go/quic-go"
)

var (
	ErrClientNotFound = errors.New("client not found")
	ErrTopicNotFound  = errors.New("topic not found")
	ErrHubClosed      = errors.New("hub is closed")
)

var bufferPool = &sync.Pool{
	New: func() any {
		return make([]byte, 1024)
	},
}

// Implements core.AxiumConnection
type ConnectionWrapper struct {
	conn quic.Connection
}

func (cw ConnectionWrapper) Close(code int, errorString string) {
	cw.conn.CloseWithError(quic.ApplicationErrorCode(code), errorString)
}

func (cw ConnectionWrapper) GetRemoteAddress() string {
	return cw.conn.RemoteAddr().String()
}

type hubOperationType int

const (
	OpRegisterClient hubOperationType = iota
	OpUnregisterClient
	OpSendMessage
	OpCreateTopic
	OpDeleteTopic
	OpSubscribe
	OpUnsubscribe
	OpPublish
)

type hubOperation struct {
	Type     hubOperationType
	ClientId string
	TopicId  string
	Client   *Client
	Topic    *Topic
	Message  []byte
	Code     int
	Response chan error
}

// Implements core.AxiumHub
type Hub struct {
	listener     *quic.Listener
	clients      map[string]*Client
	topics       map[string]*Topic
	operations   chan hubOperation
	ctx          context.Context
	cancel       context.CancelFunc
	clientMu     sync.RWMutex
	topicMu      sync.RWMutex
	onConnect    func(func(string), core.AxiumConnection)
	onDisconnect func(string)
	onMessage    func(string, []byte)
	closed       bool
	closedMu     sync.RWMutex
}

func NewQuicHub(address string, tlsConf *tls.Config, config *quic.Config) (*Hub, error) {
	h := &Hub{
		clients:    make(map[string]*Client),
		topics:     make(map[string]*Topic),
		operations: make(chan hubOperation, 100),
	}

	h.onConnect = func(accept func(string), conn core.AxiumConnection) {
		fmt.Println("New client connected")
		accept(uuid.New().String())
	}

	h.onDisconnect = func(id string) {
		fmt.Printf("Client %s disconnected\n", id)
	}

	h.onMessage = func(id string, b []byte) {
		fmt.Printf("Received message by client %s: %s\n", id, b)
	}

	ctx, cancel := context.WithCancel(context.Background())
	h.ctx = ctx
	h.cancel = cancel

	go h.run()

	listener, err := quic.ListenAddr(address, tlsConf, config)
	if err != nil {
		return nil, err
	}
	h.listener = listener

	go h.acceptConnections()

	return h, nil
}

func (h *Hub) acceptConnections() {
	for {
		conn, err := h.listener.Accept(h.ctx)
		if err != nil {
			select {
			case <-h.ctx.Done():
				return
			default:
				fmt.Printf("failed accepting connection: %s\n", err)
				continue
			}
		}

		accept := func(id string) {
			h.registerClient(id, conn)
		}
		connWrapper := ConnectionWrapper{conn: conn}
		go h.onConnect(accept, connWrapper)
	}
}

func (h *Hub) run() {
	for {
		select {
		case <-h.ctx.Done():
			return

		case op := <-h.operations:
			h.handleOperation(op)
		}
	}
}

func (h *Hub) handleOperation(op hubOperation) {
	var err error

	switch op.Type {
	case OpRegisterClient:
		h.clientMu.Lock()
		h.clients[op.Client.id] = op.Client
		h.clientMu.Unlock()
		go op.Client.ReadPump(h)
		go op.Client.WritePump(h)

	case OpUnregisterClient:
		h.clientMu.Lock()
		if client, ok := h.clients[op.ClientId]; ok {
			delete(h.clients, op.ClientId)
			close(client.send)
			client.conn.CloseWithError(quic.ApplicationErrorCode(op.Code), string(op.Message))
		}
		h.clientMu.Unlock()

	case OpSendMessage:
		h.clientMu.RLock()
		client, exists := h.clients[op.ClientId]
		h.clientMu.RUnlock()

		if !exists {
			err = ErrClientNotFound
		} else {
			select {
			case client.send <- op.Message:
				// Message sent successfully
			case <-time.After(time.Second):
				err = fmt.Errorf("timeout sending message to client %s", op.ClientId)
			}
		}

	case OpCreateTopic:
		h.topicMu.Lock()
		h.topics[op.Topic.id] = op.Topic
		h.topicMu.Unlock()
		go op.Topic.run(h.ctx)

	case OpDeleteTopic:
		h.topicMu.Lock()
		if topic, ok := h.topics[op.TopicId]; ok {
			delete(h.topics, op.TopicId)
			close(topic.subscribe)
			close(topic.unsubscribe)
			close(topic.publish)
		}
		h.topicMu.Unlock()

	case OpSubscribe:
		h.topicMu.RLock()
		topic, topicExists := h.topics[op.TopicId]
		h.topicMu.RUnlock()

		if !topicExists {
			err = ErrTopicNotFound
		} else {
			h.clientMu.RLock()
			client, clientExists := h.clients[op.ClientId]
			h.clientMu.RUnlock()

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

	case OpUnsubscribe:
		h.topicMu.RLock()
		topic, topicExists := h.topics[op.TopicId]
		h.topicMu.RUnlock()

		if !topicExists {
			err = ErrTopicNotFound
		} else {
			h.clientMu.RLock()
			client, clientExists := h.clients[op.ClientId]
			h.clientMu.RUnlock()

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

	case OpPublish:
		h.topicMu.RLock()
		topic, exists := h.topics[op.TopicId]
		h.topicMu.RUnlock()

		if !exists {
			err = ErrTopicNotFound
		} else {
			select {
			case topic.publish <- op.Message:
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

func (h *Hub) isClosed() bool {
	h.closedMu.RLock()
	defer h.closedMu.RUnlock()
	return h.closed
}

func (h *Hub) Close() error {
	h.closedMu.Lock()
	h.closed = true
	h.closedMu.Unlock()

	h.cancel()
	return h.listener.Close()
}

func (h *Hub) registerClient(clientId string, conn quic.Connection) error {
	if h.isClosed() {
		return ErrHubClosed
	}

	client := NewClient(clientId, conn)
	response := make(chan error, 1)

	h.operations <- hubOperation{
		Type:     OpRegisterClient,
		Client:   client,
		Response: response,
	}

	return <-response
}

func (h *Hub) CloseClient(id string, code int, reason string) error {
	if h.isClosed() {
		return ErrHubClosed
	}

	response := make(chan error, 1)

	h.operations <- hubOperation{
		Type:     OpUnregisterClient,
		ClientId: id,
		Code:     code,
		Message:  []byte(reason),
		Response: response,
	}

	return <-response
}

func (h *Hub) Send(clientId string, message []byte) error {
	if h.isClosed() {
		return ErrHubClosed
	}

	response := make(chan error, 1)

	h.operations <- hubOperation{
		Type:     OpSendMessage,
		ClientId: clientId,
		Message:  message,
		Response: response,
	}

	return <-response
}

func (h *Hub) CreateTopic(id string) error {
	if h.isClosed() {
		return ErrHubClosed
	}

	topic := NewTopic(id)
	response := make(chan error, 1)

	h.operations <- hubOperation{
		Type:     OpCreateTopic,
		Topic:    topic,
		Response: response,
	}

	return <-response
}

func (h *Hub) DeleteTopic(id string) error {
	if h.isClosed() {
		return ErrHubClosed
	}

	response := make(chan error, 1)

	h.operations <- hubOperation{
		Type:     OpDeleteTopic,
		TopicId:  id,
		Response: response,
	}

	return <-response
}

func (h *Hub) Subscribe(clientId string, topicId string) error {
	if h.isClosed() {
		return ErrHubClosed
	}

	response := make(chan error, 1)

	h.operations <- hubOperation{
		Type:     OpSubscribe,
		ClientId: clientId,
		TopicId:  topicId,
		Response: response,
	}

	return <-response
}

func (h *Hub) Unsubscribe(clientId string, topicId string) error {
	if h.isClosed() {
		return ErrHubClosed
	}

	response := make(chan error, 1)

	h.operations <- hubOperation{
		Type:     OpUnsubscribe,
		ClientId: clientId,
		TopicId:  topicId,
		Response: response,
	}

	return <-response
}

func (h *Hub) Publish(topicId string, message []byte) error {
	if h.isClosed() {
		return ErrHubClosed
	}

	response := make(chan error, 1)

	h.operations <- hubOperation{
		Type:     OpPublish,
		TopicId:  topicId,
		Message:  message,
		Response: response,
	}

	return <-response
}

func (h *Hub) GetClientIds() []string {
	h.clientMu.RLock()
	defer h.clientMu.RUnlock()

	ids := make([]string, 0, len(h.clients))
	for id := range h.clients {
		ids = append(ids, id)
	}
	return ids
}

func (h *Hub) GetTopicIds() []string {
	h.topicMu.RLock()
	defer h.topicMu.RUnlock()

	ids := make([]string, 0, len(h.topics))
	for id := range h.topics {
		ids = append(ids, id)
	}
	return ids
}

func (h *Hub) GetClientIdsOfTopic(topicId string) ([]string, error) {
	h.topicMu.RLock()
	defer h.topicMu.RUnlock()

	topic, exists := h.topics[topicId]
	if !exists {
		return nil, ErrTopicNotFound
	}
	return topic.getClientIds(), nil
}

func (h *Hub) OnConnect(fn func(func(string), core.AxiumConnection)) {
	h.onConnect = fn
}

func (h *Hub) OnDisconnect(fn func(string)) {
	h.onDisconnect = fn
}

func (h *Hub) OnMessage(fn func(string, []byte)) {
	h.onMessage = fn
}
