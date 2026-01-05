package ecs

import (
	"fmt"
	"iter"
	"reflect"
	"sync"
)

// World contains the state of an ECSEngine.
type World struct {
	entities      map[Entity]struct{}
	stores        map[reflect.Type]typedStore
	storesById    map[uint16]typedStore
	singletons    map[reflect.Type]any
	messages      map[reflect.Type]typedMessageStore
	autoComponent uint16
}

// NewWorld creates a new World.
func NewWorld() *World {
	return &World{
		entities:      make(map[Entity]struct{}),
		stores:        make(map[reflect.Type]typedStore),
		storesById:    make(map[uint16]typedStore),
		singletons:    make(map[reflect.Type]any),
		messages:      make(map[reflect.Type]typedMessageStore),
		autoComponent: 10_000,
	}
}

// ==================================================================
// Entities
// ==================================================================

// Entity represents an ECS entity.
type Entity uint64

func (w *World) entityExists(e Entity) bool {
	_, exists := w.entities[e]
	return exists
}

// ==================================================================
// Commands
// ==================================================================

func (w *World) processCommands(commands []Command) {
	for _, cmd := range commands {
		switch cmd.Op {
		case CreateEntityCommand:
			w.createEntity(cmd.Entity, cmd.Values)
		case DestroyEntityCommand:
			w.destroyEntity(cmd.Entity)
		case AddComponentToEntity:
			w.addComponent(cmd.Entity, cmd.Type, cmd.Value)
		case RemoveComponentFromEntity:
			w.removeComponent(cmd.Entity, cmd.Type)
		}
	}
}

func (w *World) createEntity(e Entity, values map[reflect.Type]any) {
	w.entities[e] = struct{}{}
	for t, v := range values {
		w.addComponent(e, t, v)
	}
}

func (w *World) destroyEntity(e Entity) {
	delete(w.entities, e)

	for _, store := range w.stores {
		if store.hasEntity(e) {
			store.remove(e)
		}
	}
}

func (w *World) addComponent(e Entity, typ reflect.Type, initial any) {
	s, ok := w.stores[typ]
	if ok {
		s.add(e, initial)
	}
}

func (w *World) removeComponent(e Entity, typ reflect.Type) {
	s, ok := w.stores[typ]
	if ok {
		s.remove(e)
	}
}

// ==================================================================
// Messages
// ==================================================================

type typedMessageStore interface {
	swap()
}

type messageStore[T any] struct {
	read      []T
	write     []T
	writeSafe []T
	mu        sync.Mutex // For swap
}

func (s *messageStore[T]) swap() {
	s.mu.Lock()
	ws := s.writeSafe
	s.writeSafe = s.writeSafe[:0]

	s.read = s.read[:0]

	s.read = append(s.read, s.write...)
	s.read = append(s.read, ws...)

	s.write = s.write[:0]
	s.mu.Unlock()
}

func registerMessage[T any](w *World) {
	t := reflect.TypeFor[T]()

	store := &messageStore[T]{
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

	store, ok := s.(*messageStore[T])
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

	store, ok := s.(*messageStore[T])
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

	store, ok := s.(*messageStore[T])
	if !ok {
		return nil
	}

	return store.read
}

func collectMessagesSafe[T any](w *World) []T {
	t := reflect.TypeFor[T]()

	s, ok := w.messages[t]
	if !ok {
		return nil
	}

	store, ok := s.(*messageStore[T])
	if !ok {
		return nil
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	return store.read
}

// ==================================================================
// Singletons
// ==================================================================

type singletonStore[T any] struct {
	data  T
	dirty bool
}

func registerSingleton[T any](w *World, initial T) {
	t := reflect.TypeFor[T]()

	store := &singletonStore[T]{
		data: initial,
	}

	w.singletons[t] = store
}

func getSingleton[T any](w *World) (*Singleton[T], bool) {
	t := reflect.TypeFor[T]()
	s := w.singletons[t]

	store, ok := s.(*singletonStore[T])
	if !ok {
		return nil, false
	}

	return &Singleton[T]{s: store}, true
}

// Singleton is a wrapper for a singleton value.
type Singleton[T any] struct {
	s *singletonStore[T]
}

// Get retrives the sigleton T.
func (s *Singleton[T]) Get() *T {
	return &s.s.data
}

// Get retrieves the sigleton T and marks it as dirty.
func (s *Singleton[T]) Mut() *T {
	s.s.dirty = true
	return &s.s.data
}

// IsDirty reports whether the singleton is dirty.
func (s *Singleton[T]) IsDirty() bool {
	return s.s.dirty
}

// ==================================================================
// Components
// ==================================================================

func (w *World) generateId(auto bool, id uint16) uint16 {
	if !auto {
		if s, ok := w.storesById[id]; ok {
			panic(fmt.Sprintf("Component with id %d already registered (%s)", id, s.Type()))
		}
		return id
	}

	var generated uint16
	for range 30_000 {
		generated = w.autoComponent
		w.autoComponent++
		if _, ok := w.storesById[generated]; !ok {
			return generated
		}
	}
	return 0
}

func registerComponent[T any](w *World, autoId bool, id uint16) {
	var z T
	t := reflect.TypeOf(z)

	if _, ok := w.stores[t]; ok {
		return
	}

	id = w.generateId(autoId, id)

	s := &store[T]{
		id:       id,
		typ:      t,
		sparse:   make(map[Entity]int),
		dirtySet: make(map[Entity]struct{}),
	}

	w.stores[t] = s
	w.storesById[id] = s
}

func getStoreFromWorld[T any](w *World) (*store[T], bool) {
	t := reflect.TypeFor[T]()

	s, ok := w.stores[t]
	if !ok {
		return nil, false
	}
	store, ok := s.(*store[T])
	return store, ok
}

type typedStore interface {
	Type() reflect.Type
	entities() iter.Seq[Entity]
	hasEntity(e Entity) bool
	add(e Entity, value any)
	remove(e Entity)
	len() int
}

type store[T any] struct {
	id       uint16
	typ      reflect.Type
	sparse   map[Entity]int
	dense    []Entity
	data     []T
	dirtySet map[Entity]struct{}
	dirty    bool
}

func (s store[T]) String() string {
	return fmt.Sprintf(
		"ComponentStore{typ:%s id:%d, len:%d}",
		s.typ,
		s.id,
		len(s.dense),
	)
}

func (s *store[T]) Type() reflect.Type {
	return s.typ
}

func (s *store[T]) add(e Entity, value any) {
	if s.hasEntity(e) {
		return
	}

	initial, ok := value.(T)
	if !ok {
		return
	}

	newIndex := len(s.data)

	s.data = append(s.data, initial)
	s.dense = append(s.dense, e)

	s.sparse[e] = newIndex
}

func (s *store[T]) remove(e Entity) {
	idx, exists := s.sparse[e]
	if !exists {
		return
	}

	lastIndex := len(s.data) - 1
	lastEntityID := s.dense[lastIndex]

	if idx != lastIndex {
		s.data[idx] = s.data[lastIndex]
		s.dense[idx] = lastEntityID

		s.sparse[lastEntityID] = idx
	}

	s.data = s.data[:lastIndex]
	s.dense = s.dense[:lastIndex]

	delete(s.sparse, e)
}

func (s *store[T]) len() int {
	return len(s.dense)
}

func (s *store[T]) entities() iter.Seq[Entity] {
	return func(yield func(Entity) bool) {
		for _, e := range s.dense {
			if !yield(e) {
				break
			}
		}
	}
}

func (s *store[T]) hasEntity(e Entity) bool {
	_, ok := s.sparse[e]
	return ok
}

func (s *store[T]) get(e Entity) *T {
	return &s.data[s.sparse[e]]
}

func (s *store[T]) mut(e Entity) *T {
	s.dirtySet[e] = struct{}{}
	return &s.data[s.sparse[e]]
}

func (s *store[T]) markDirty() {
	s.dirty = true
}

func (s *store[T]) resetDirty() {
	s.dirty = false
	if len(s.dirtySet) > 0 {
		s.dirtySet = make(map[Entity]struct{})
	}
}

func (s *store[T]) getDirtyComps() map[Entity]T {
	comps := make(map[Entity]T)

	if s.dirty {
		for e, idx := range s.sparse {
			comps[e] = s.data[idx]
		}
		return comps
	}

	for e := range s.dirtySet {
		idx, ok := s.sparse[e]
		if !ok {
			continue
		}
		comps[e] = s.data[idx]
	}
	return comps
}
