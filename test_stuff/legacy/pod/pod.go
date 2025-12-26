package pod

import (
	"context"
	"sync"
	"time"

	"github.com/QYUbit/Axium/pkg/transport"
)

type Session interface {
	Set(key string, value any)
	Get(key string) (any, bool)
}

type IdGenerator func() string

type Pod struct {
	transport   transport.Transport
	idGenerator IdGenerator
}

type Options struct {
	Transport   transport.Transport
	IdGenerator IdGenerator
}

type Message struct {
	Action  uint8
	Payload []byte
}

type Serializer interface {
	Deserialize(data []byte) (Message, error)
	Serialize(msg Message) ([]byte, error)
}

func NewPod(o Options) *Pod {
	p := &Pod{
		transport:   o.Transport,
		idGenerator: o.IdGenerator,
	}

	return p
}

func (p *Pod) handleEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.transport.Connections():
		case <-p.transport.Disconnections():
		case <-p.transport.Messages():
		}
	}
}

func (p *Pod) Start(ctx context.Context) error {
	if err := p.transport.Start(ctx); err != nil {
		return err
	}

	go p.handleEvents(ctx)
	return nil
}

type Event interface {
	Type() string
}

type EventManager[T Event] struct {
	timeout       time.Duration
	eventChan     chan T
	eventHandlers map[string]func(T)
	mu            sync.RWMutex
}

func NewEventManager[T Event](timeout time.Duration) *EventManager[T] {
	return &EventManager[T]{
		timeout:       timeout,
		eventChan:     make(chan T),
		eventHandlers: make(map[string]func(T)),
	}
}

func (em *EventManager[T]) Dispatch(e T) {
	em.mu.RLock()
	handler, ok := em.eventHandlers[e.Type()]
	em.mu.RUnlock()

	if ok {
		handler(e)
	}

	select {
	case em.eventChan <- e:
	case <-time.After(em.timeout):
		return
	}
}

func (em *EventManager[T]) Events() <-chan T {
	return em.eventChan
}

func (em *EventManager[T]) On(typ string, cb func(e T)) {
	em.mu.Lock()
	em.eventHandlers[typ] = cb
	em.mu.Unlock()
}
