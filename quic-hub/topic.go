package quichub

import (
	"context"
	"sync"
)

type Topic struct {
	id          string
	clients     map[string]*Client
	subscribe   chan *Client
	unsubscribe chan *Client
	publish     chan []byte
	mu          sync.RWMutex
}

func NewTopic(id string) *Topic {
	return &Topic{
		id:          id,
		clients:     make(map[string]*Client),
		subscribe:   make(chan *Client),
		unsubscribe: make(chan *Client),
		publish:     make(chan []byte),
	}
}

func (t *Topic) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case client := <-t.subscribe:
			t.mu.Lock()
			t.clients[client.id] = client
			t.mu.Unlock()

		case client := <-t.unsubscribe:
			t.mu.Lock()
			delete(t.clients, client.id)
			t.mu.Unlock()

		case message := <-t.publish:
			t.mu.RLock()
			for _, client := range t.clients {
				client.send <- message
			}
			t.mu.RUnlock()
		}
	}
}

func (t *Topic) getClientIds() []string {
	t.mu.RLock()
	ids := make([]string, 0, len(t.clients))
	for id := range t.clients {
		ids = append(ids, id)
	}
	t.mu.RUnlock()
	return ids
}
