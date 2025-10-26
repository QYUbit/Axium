package quichub

import (
	"fmt"
	"sync"
	"time"
)

type topic struct {
	id          string
	clients     map[string]*client
	subscribe   chan *client
	unsubscribe chan *client
	publish     chan message
	mu          sync.RWMutex
}

func newTopic(id string) *topic {
	return &topic{
		id:          id,
		clients:     make(map[string]*client),
		subscribe:   make(chan *client, 10),
		unsubscribe: make(chan *client, 10),
		publish:     make(chan message, 100),
	}
}

func (t *topic) run(transport *QuicTransport) {
	defer func() {
		if err := recover(); err != nil {
			transport.onError(fmt.Errorf("Topic %s panic: %v\n", t.id, err))
		}
	}()

	for {
		select {
		case <-transport.ctx.Done():
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
			clients := make([]*client, 0, len(t.clients))
			for _, client := range t.clients {
				clients = append(clients, client)
			}
			t.mu.RUnlock()

			for _, client := range clients {
				select {
				case client.send <- message:
				case <-time.After(100 * time.Millisecond):
					transport.onError(fmt.Errorf("Timeout sending to client %s in topic %s\n", client.id, t.id))
				}
			}
		}
	}
}

func (t *topic) getClientIds() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	ids := make([]string, 0, len(t.clients))
	for id := range t.clients {
		ids = append(ids, id)
	}
	return ids
}
