package main

import (
	"reflect"
	"sync"
)

type typedQueue interface {
	Reset()
}

// TODO Optinally change to lockless version without Unregister in the future

type eventBus struct {
	mu     sync.RWMutex
	queues map[reflect.Type]typedQueue
}

func NewEventBus() *eventBus {
	return &eventBus{
		queues: make(map[reflect.Type]typedQueue),
	}
}

func RegisterEvent[T any](bus *eventBus) *eventQueue[T] {
	var zero T
	t := reflect.TypeOf(zero)

	q := &eventQueue[T]{}

	bus.mu.Lock()
	bus.queues[t] = q
	bus.mu.Unlock()

	return q
}

func UnregisterEvent[T any](bus *eventBus) {
	var zero T
	t := reflect.TypeOf(zero)

	bus.mu.RLock()
	q, ok := bus.queues[t]
	bus.mu.RUnlock()

	if !ok {
		return
	}
	q.Reset()

	bus.mu.Lock()
	delete(bus.queues, t)
	bus.mu.Unlock()
}

func GetQueueFromBus[T any](bus *eventBus) (*eventQueue[T], bool) {
	var zero T
	t := reflect.TypeOf(zero)

	bus.mu.RLock()
	q, ok := bus.queues[t]
	bus.mu.RUnlock()

	return q.(*eventQueue[T]), ok
}

func (bus *eventBus) Reset() {
	bus.mu.RLock()
	defer bus.mu.RUnlock()

	for _, q := range bus.queues {
		q.Reset()
	}
}

type eventQueue[T any] struct {
	mu     sync.Mutex
	events []T
}

func (b *eventQueue[T]) Push(e T) {
	b.mu.Lock()
	b.events = append(b.events, e)
	b.mu.Unlock()
}

func (b *eventQueue[T]) Pull() []T {
	b.mu.Lock()
	out := b.events
	b.events = nil
	b.mu.Unlock()
	return out
}

func (b *eventQueue[T]) Reset() {
	b.mu.Lock()
	b.events = nil
	b.mu.Unlock()
}
