package ecs

import (
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

type EntityId uint64

type ComponentType string

type Component interface {
	Type() ComponentType
}

type System interface {
	Update(dt time.Duration, entities []EntityId, world *World)
	Components() []ComponentType
	Priority() int
}

type Entity struct {
	components map[ComponentType]Component
	mu         sync.RWMutex
	isDeleted  bool
}

func (e *Entity) AddComponent(c Component) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.components[c.Type()] = c
}

func (e *Entity) RemoveComponent(ct ComponentType) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.components, ct)
}

func (e *Entity) GetComponent(ct ComponentType) (Component, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	c, ok := e.components[ct]
	return c, ok
}

func (e *Entity) HasComponents(types ...ComponentType) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, t := range types {
		if _, ok := e.components[t]; !ok {
			return false
		}
	}
	return true
}

type World struct {
	nextEntityId  uint64
	entities      map[EntityId]*Entity
	compIndex     map[ComponentType][]EntityId
	mu            sync.RWMutex
	systems       []System
	dirtyEntities map[EntityId]bool
	syncMu        sync.RWMutex
	eventHandlers map[string][]EventHandler
	eventQueue    []Event
	eventMu       sync.Mutex
}

func NewWorld() *World {
	return &World{
		entities:      make(map[EntityId]*Entity),
		compIndex:     make(map[ComponentType][]EntityId),
		dirtyEntities: make(map[EntityId]bool),
		eventHandlers: make(map[string][]EventHandler),
	}
}

func (w *World) CreateEntity() *Entity {
	id := EntityId(atomic.AddUint64(&w.nextEntityId, 1))

	entity := &Entity{
		components: make(map[ComponentType]Component),
	}

	w.mu.Lock()
	w.entities[id] = entity
	w.mu.Unlock()

	w.MarkDirty(id)
	w.EmitEvent(Event{Type: "entity_created", EntityId: id})

	return entity
}

func (w *World) DestroyEntity(id EntityId) bool {
	w.mu.Lock()
	entity, ok := w.entities[id]
	if !ok {
		w.mu.Unlock()
		return false
	}

	entity.isDeleted = true
	delete(w.entities, id)

	for componentType := range entity.components {
		w.removeFromComponentIndex(componentType, id)
	}
	w.mu.Unlock()

	w.MarkDirty(id)
	w.EmitEvent(Event{Type: "entity_destroyed", EntityId: id})
	return true
}

func (w *World) GetEntity(id EntityId) (*Entity, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	entity, ok := w.entities[id]
	return entity, ok
}

func (w *World) GetAllEntities() {}

func (w *World) AddComponent(eid EntityId, c Component) bool {
	entity, ok := w.GetEntity(eid)
	if !ok {
		return false
	}

	entity.AddComponent(c)

	w.mu.Lock()
	w.addToComponentIndex(c.Type(), eid)
	w.mu.Unlock()

	w.MarkDirty(eid)
	return true
}

func (w *World) RemoveComponent(eid EntityId, ct ComponentType) bool {
	entity, ok := w.GetEntity(eid)
	if !ok {
		return false
	}

	entity.RemoveComponent(ct)

	w.mu.Lock()
	w.removeFromComponentIndex(ct, eid)
	w.mu.Unlock()

	w.MarkDirty(eid)
	return true
}

func (w *World) addToComponentIndex(ct ComponentType, id EntityId) {
	entities := w.compIndex[ct]
	if slices.Contains(entities, id) {
		return
	}
	w.compIndex[ct] = append(entities, id)
}

func (w *World) removeFromComponentIndex(ct ComponentType, id EntityId) {
	entities := w.compIndex[ct]
	for i, eid := range entities {
		if eid == id {
			w.compIndex[ct] = append(entities[:i], entities[i+1:]...)
			return
		}
	}
}

func (w *World) RegisterSystem(s System) {
	w.mu.Lock()
	defer w.mu.Unlock()

	inserted := false
	for i, existing := range w.systems {
		if s.Priority() > existing.Priority() {
			w.systems = append(w.systems[:i], append([]System{s}, w.systems[i:]...)...)
			inserted = true
			break
		}
	}

	if !inserted {
		w.systems = append(w.systems, s)
	}
}

func (w *World) Update(dt time.Duration) {
	w.mu.RLock()
	systems := make([]System, len(w.systems))
	copy(systems, w.systems)
	w.mu.RUnlock()

	for _, system := range systems {
		entities := w.Query(system.Components()...)
		system.Update(dt, entities, w)
	}

	// Events verarbeiten
	w.ProcessEvents()
}

func (w *World) Query(types ...ComponentType) []EntityId {
	if len(types) == 0 {
		return []EntityId{}
	}

	w.mu.RLock()
	defer w.mu.RUnlock()

	var result []EntityId
	minSize := -1
	var minType ComponentType

	for _, t := range types {
		size := len(w.compIndex[t])
		if minSize == -1 || size < minSize {
			minSize = size
			minType = t
		}
	}

	if minSize == -1 || minSize == 0 {
		return []EntityId{}
	}

	for _, id := range w.compIndex[minType] {
		entity, ok := w.entities[id]
		if !ok || entity.isDeleted {
			continue
		}

		if entity.HasComponents(types...) {
			result = append(result, id)
		}
	}

	return result
}

func (w *World) MarkDirty(id EntityId) {
	w.syncMu.Lock()
	defer w.syncMu.Unlock()
	w.dirtyEntities[id] = true
}

func (w *World) GetDirtyEntities() []EntityId {
	w.syncMu.Lock()
	defer w.syncMu.Unlock()

	result := make([]EntityId, 0, len(w.dirtyEntities))
	for id := range w.dirtyEntities {
		result = append(result, id)
	}
	return result
}

func (w *World) ClearDirty() {
	w.syncMu.Lock()
	defer w.syncMu.Unlock()
	w.dirtyEntities = make(map[EntityId]bool)
}

type Event struct {
	Type     string
	EntityId EntityId
	Data     any
}

type EventHandler func(Event)

func (w *World) RegisterEventHandler(eventType string, handler EventHandler) {
	w.eventMu.Lock()
	defer w.eventMu.Unlock()
	w.eventHandlers[eventType] = append(w.eventHandlers[eventType], handler)
}

func (w *World) EmitEvent(event Event) {
	w.eventMu.Lock()
	w.eventQueue = append(w.eventQueue, event)
	w.eventMu.Unlock()
}

func (w *World) ProcessEvents() {
	w.eventMu.Lock()
	events := make([]Event, len(w.eventQueue))
	copy(events, w.eventQueue)
	w.eventQueue = w.eventQueue[:0]
	w.eventMu.Unlock()

	for _, event := range events {
		if handlers, ok := w.eventHandlers[event.Type]; ok {
			for _, handler := range handlers {
				handler(event)
			}
		}
	}
}
