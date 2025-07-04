package quichub

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/quic-go/quic-go"
)

var (
	ErrClientNotFound = errors.New("can not find client")
	ErrTopicNotFound  = errors.New("can not find topic")
)

var bufferPool = &sync.Pool{
	New: func() any {
		return make([]byte, 1024)
	},
}

type ClientHub interface {
	RegisterClient(string, any) error
	UnregisterClient(string) error
	GetClientIds() []string
	Send(string, []byte) error
	OnConnect(any) error
	OnMessage(func(string, []byte))
}

type PubSubHub interface {
	ClientHub
	Publish(string, []byte) error
	Subscribe(string, string) error
	Unsubscribe(string, string) error
	CreateTopic(string) error
	DeleteTopic(string) error
	GetTopicIds() []string
	GetClientIdsOfTopic(string) ([]string, error)
}

func a(c PubSubHub) {}

func A() {
	h, _ := NewQuicHub("localhost:8080", nil, nil)
	a(h)
}

type Hub struct {
	listener    *quic.Listener
	clients     map[string]*Client
	topics      map[string]*Topic
	register    chan *Client
	unregister  chan *Client
	createTopic chan *Topic
	deleteTopic chan *Topic
	ctx         context.Context
	cancel      context.CancelFunc
	clientMu    sync.RWMutex
	topicMu     sync.RWMutex
	onConnect   func(quic.Connection)
	onMessage   func(string, []byte)
}

func NewQuicHub(address string, tlsConf *tls.Config, config *quic.Config) (*Hub, error) {
	h := &Hub{
		clients:     make(map[string]*Client),
		topics:      make(map[string]*Topic),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		createTopic: make(chan *Topic),
		deleteTopic: make(chan *Topic),
	}

	h.onConnect = func(conn quic.Connection) {
		h.RegisterClient(uuid.New().String(), conn)
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

	go func() {
		for {
			conn, err := listener.Accept(context.Background())
			if err != nil {
				fmt.Printf("failed accepting connection: %s\n", err)
				continue
			}
			go h.onConnect(conn)
		}
	}()

	return h, nil
}

func (h *Hub) run() {
	for {
		select {
		case <-h.ctx.Done():
			return

		case client := <-h.register:
			h.clientMu.Lock()
			h.clients[client.id] = client
			h.clientMu.Unlock()

		case client := <-h.unregister:
			h.clientMu.Lock()
			if _, ok := h.clients[client.id]; ok {
				delete(h.clients, client.id)
				close(client.send)
			}
			h.clientMu.Unlock()

		case topic := <-h.createTopic:
			h.topicMu.Lock()
			h.topics[topic.id] = topic
			h.topicMu.Unlock()

		case topic := <-h.deleteTopic:
			h.topicMu.Lock()
			if _, ok := h.topics[topic.id]; ok {
				delete(h.topics, topic.id)
				close(topic.subscribe)
				close(topic.unsubscribe)
				close(topic.publish)
			}
			h.topicMu.Unlock()
		}
	}
}

func (h *Hub) Close() error {
	h.cancel()
	return h.listener.Close()
}

func (h *Hub) RegisterClient(clientId string, anyConn any) error {
	conn, ok := anyConn.(quic.Connection)
	if !ok {
		return fmt.Errorf("conn has to be of type quic.Connection, got: %T", anyConn)
	}
	client := NewClient(clientId, conn)
	h.register <- client
	go client.ReadPump(h)
	go client.WritePump(h)
	return nil
}

func (h *Hub) UnregisterClient(id string) error {
	h.clientMu.RLock()
	client, exists := h.clients[id]
	if !exists {
		return ErrClientNotFound
	}
	h.clientMu.RUnlock()
	h.unregister <- client
	return client.conn.CloseWithError(0, "closed")
}

func (h *Hub) Send(clientId string, message []byte) error {
	h.clientMu.RLock()
	defer h.clientMu.RUnlock()

	client, exists := h.clients[clientId]
	if !exists {
		return ErrClientNotFound
	}

	client.send <- message
	return nil
}

func (h *Hub) Publish(topicId string, message []byte) error {
	h.topicMu.RLock()
	defer h.topicMu.RUnlock()

	topic, exists := h.topics[topicId]
	if !exists {
		return ErrTopicNotFound
	}

	topic.publish <- message
	return nil
}

func (h *Hub) CreateTopic(id string) error {
	topic := NewTopic(id)
	go topic.run(h.ctx)
	h.createTopic <- topic
	return nil
}

func (h *Hub) DeleteTopic(id string) error {
	h.topicMu.RLock()
	defer h.topicMu.RUnlock()

	topic, exists := h.topics[id]
	if !exists {
		return ErrTopicNotFound
	}

	h.deleteTopic <- topic
	return nil
}

func (h *Hub) Subscribe(clientId string, topicId string) error {
	h.topicMu.RLock()
	defer h.topicMu.RUnlock()
	topic, exists := h.topics[topicId]
	if !exists {
		return ErrTopicNotFound
	}

	h.clientMu.RLock()
	defer h.clientMu.RUnlock()
	client, exists := h.clients[clientId]
	if !exists {
		return ErrClientNotFound
	}

	topic.subscribe <- client
	return nil
}

func (h *Hub) Unsubscribe(clientId string, topicId string) error {
	h.topicMu.RLock()
	defer h.topicMu.RUnlock()
	topic, exists := h.topics[topicId]
	if !exists {
		return ErrTopicNotFound
	}

	h.clientMu.RLock()
	defer h.clientMu.RUnlock()
	client, exists := h.clients[clientId]
	if !exists {
		return ErrClientNotFound
	}

	topic.unsubscribe <- client
	return nil
}

func (h *Hub) GetClientIds() []string {
	h.clientMu.RLock()
	defer h.clientMu.RUnlock()
	var ids []string
	for id := range h.clients {
		ids = append(ids, id)
	}
	return ids
}

func (h *Hub) GetTopicIds() []string {
	h.topicMu.RLock()
	defer h.topicMu.RUnlock()
	var ids []string
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

func (h *Hub) OnConnect(anyFn any) error {
	fn, ok := anyFn.(func(quic.Connection))
	if !ok {
		return fmt.Errorf("conn has to be of type quic.Connection, got: %T", anyFn)
	}
	h.onConnect = fn
	return nil
}

func (h *Hub) OnMessage(fn func(string, []byte)) {
	h.onMessage = fn
}
