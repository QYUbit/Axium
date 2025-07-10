package quichub

import (
	"context"
	"fmt"
	"sync"
	"time"
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
		subscribe:   make(chan *Client, 10),
		unsubscribe: make(chan *Client, 10),
		publish:     make(chan []byte, 100),
	}
}

func (t *Topic) run(ctx context.Context) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Printf("Topic %s panic: %v\n", t.id, err)
		}
	}()

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
			clients := make([]*Client, 0, len(t.clients))
			for _, client := range t.clients {
				clients = append(clients, client)
			}
			t.mu.RUnlock()

			for _, client := range clients {
				select {
				case client.send <- message:
				case <-time.After(100 * time.Millisecond):
					fmt.Printf("Timeout sending to client %s in topic %s\n", client.id, t.id)
				}
			}
		}
	}
}

func (t *Topic) getClientIds() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	ids := make([]string, 0, len(t.clients))
	for id := range t.clients {
		ids = append(ids, id)
	}
	return ids
}
