package ecs

import (
	"iter"
	"reflect"
	"sync"
)

type EntityID uint64
type ComponentID uint16

type TypeID uint16

type Component interface {
	Id() ComponentID
}

type World struct {
	entities   map[EntityID]struct{}
	stores     map[reflect.Type]TypedStore
	storesById map[ComponentID]TypedStore
	singletons map[reflect.Type]any
	messages   map[reflect.Type]TypedMessageStore
}

func NewWorld() *World {
	return &World{
		entities:   make(map[EntityID]struct{}),
		stores:     make(map[reflect.Type]TypedStore),
		storesById: make(map[ComponentID]TypedStore),
		singletons: make(map[reflect.Type]any),
		messages:   make(map[reflect.Type]TypedMessageStore),
	}
}

// ==================================================================
// Commands
// ==================================================================

func (w *World) processCommands(commands []Command) {
	for _, cmd := range commands {
		switch cmd.Op {
		case CreateEntityCommand:
			w.createEntity(cmd.EntityId)
		case DestroyEntityCommand:
			w.destroyEntity(cmd.EntityId)
		case AddComponentToEntity:
			w.addComponent(cmd.EntityId, cmd.Type, cmd.Value)
		case RemoveComponentFromEntity:
			w.removeComponent(cmd.EntityId, cmd.Type)
		}
	}
}

func (w *World) createEntity(id EntityID) {
	w.entities[id] = struct{}{}
}

func (w *World) destroyEntity(id EntityID) {
	delete(w.entities, id)

	for _, store := range w.stores {
		if store.HasEntity(id) {
			store.Remove(id)
		}
	}
}

func (w *World) addComponent(id EntityID, typ reflect.Type, initial any) {
	s, ok := w.stores[typ]
	if ok {
		s.Add(id, initial)
	}
}

func (w *World) removeComponent(id EntityID, typ reflect.Type) {
	s, ok := w.stores[typ]
	if ok {
		s.Remove(id)
	}
}

// ==================================================================
// Messages
// ==================================================================

type TypedMessageStore interface {
	swap()
}

type MessageStore[T any] struct {
	read      []T
	write     []T
	writeSafe []T
	mu        sync.Mutex // For writeSafe
}

func (s *MessageStore[T]) swap() {
	s.mu.Lock()
	ws := s.writeSafe
	s.writeSafe = s.writeSafe[:0]
	s.mu.Unlock()

	s.read = s.read[:0]

	s.read = append(s.read, s.write...)
	s.read = append(s.read, ws...)

	s.write = s.write[:0]

}

func registerMessage[T Component](w *World) {
	t := reflect.TypeFor[T]()

	store := &MessageStore[T]{
		read:      make([]T, 0),
		write:     make([]T, 0),
		writeSafe: make([]T, 0),
	}

	w.messages[t] = store
}

func pushMessage[T any](w *World, msg T) {
	t := reflect.TypeFor[T]()

	s, ok := w.messages[t]
	if !ok {
		return
	}

	store, ok := s.(*MessageStore[T])
	if ok {
		store.write = append(store.write, msg)
	}
}

func pushMessageSafe[T any](w *World, msg T) {
	t := reflect.TypeFor[T]()

	s, ok := w.messages[t]
	if !ok {
		return
	}

	store, ok := s.(*MessageStore[T])
	if ok {
		store.mu.Lock()
		store.write = append(store.write, msg)
		store.mu.Unlock()
	}
}

func collectMessages[T any](w *World) []T {
	t := reflect.TypeFor[T]()

	s, ok := w.messages[t]
	if !ok {
		return nil
	}

	store, ok := s.(*MessageStore[T])
	if !ok {
		return nil
	}

	return store.read
}

// ==================================================================
// Singletons
// ==================================================================

type SingletonStore[T any] struct {
	data  T
	dirty bool
}

func registerSingleton[T Component](w *World, initial T) {
	t := reflect.TypeFor[T]()

	store := &SingletonStore[T]{
		data: initial,
	}

	w.singletons[t] = store
}

func getSingleton[T any](w *World) *T {
	t := reflect.TypeFor[T]()
	s := w.singletons[t]

	store, ok := s.(*SingletonStore[T])
	if !ok {
		return nil
	}

	return &store.data
}

func getMutableSingleton[T any](w *World) *T {
	t := reflect.TypeFor[T]()
	s := w.singletons[t]

	store, ok := s.(*SingletonStore[T])
	if !ok {
		return nil
	}

	store.dirty = true
	return &store.data
}

func getStaticSingleton[T any](w *World) T {
	t := reflect.TypeFor[T]()
	s := w.singletons[t]

	store, ok := s.(*SingletonStore[T])
	if !ok {
		var zero T
		return zero
	}

	return store.data
}

// ==================================================================
// Components
// ==================================================================

func registerComponent[T Component](w *World) {
	var z T
	t := reflect.TypeOf(z)

	s := &Store[T]{
		typ:    t,
		sparse: make(map[EntityID]int),
	}

	w.stores[t] = s
	w.storesById[z.Id()] = s
}

func getStoreFromWorld[T any](w *World) (*Store[T], bool) {
	t := reflect.TypeFor[T]()

	s, ok := w.stores[t]
	if !ok {
		return nil, false
	}
	store, ok := s.(*Store[T])
	return store, ok
}

type TypedStore interface {
	GetEntities() []EntityID
	Entities() iter.Seq[EntityID]
	HasEntity(id EntityID) bool
	Add(id EntityID, value any)
	Remove(id EntityID)
	Len() int
}

type Store[T any] struct {
	typ    reflect.Type
	sparse map[EntityID]int
	dense  []EntityID
	data   []T
	dirty  []bool
}

func (s *Store[T]) Add(id EntityID, value any) {
	if s.HasEntity(id) {
		return
	}

	initial, ok := value.(T)
	if !ok {
		return
	}

	newIndex := len(s.data)

	s.data = append(s.data, initial)
	s.dense = append(s.dense, id)

	s.dirty = append(s.dirty, false)

	s.sparse[id] = newIndex
}

func (s *Store[T]) Remove(id EntityID) {
	idx, exists := s.sparse[id]
	if !exists {
		return
	}

	lastIndex := len(s.data) - 1
	lastEntityID := s.dense[lastIndex]

	if idx != lastIndex {
		s.data[idx] = s.data[lastIndex]
		s.dense[idx] = lastEntityID
		s.dirty[idx] = s.dirty[lastIndex]

		s.sparse[lastEntityID] = idx
	}

	s.data = s.data[:lastIndex]
	s.dense = s.dense[:lastIndex]
	s.dirty = s.dirty[:lastIndex]

	delete(s.sparse, id)
}

func (s *Store[T]) GetEntities() []EntityID {
	return s.dense
}

func (s *Store[T]) Entities() iter.Seq[EntityID] {
	return func(yield func(EntityID) bool) {
		for _, id := range s.dense {
			if !yield(id) {
				break
			}
		}
	}
}

func (s *Store[T]) Len() int {
	return len(s.dense)
}

func (s *Store[T]) HasEntity(id EntityID) bool {
	_, ok := s.sparse[id]
	return ok
}

func (s *Store[T]) Get(id EntityID) *T {
	return &s.data[s.sparse[id]]
}

func (s *Store[T]) GetMutable(id EntityID) *T {
	idx := s.sparse[id]
	s.dirty[idx] = true
	return &s.data[idx]
}

func (s *Store[T]) GetStatic(id EntityID) T {
	return s.data[s.sparse[id]]
}

func (s *Store[T]) MarkDirty(id EntityID) {
	idx := s.sparse[id]
	s.dirty[idx] = true
}
