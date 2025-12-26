package state

import (
	"sync"
	"sync/atomic"
)

type Entity uint64

type Component interface {
	Serialize() ([]byte, error)
}

type entity struct {
	comps map[string]*comp
	mu    sync.RWMutex
}

func newEntity() *entity {
	return &entity{comps: make(map[string]*comp)}
}

func (e *entity) get(name string) (*comp, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	comp, exists := e.comps[name]
	return comp, exists
}

func (e *entity) set(name string, comp *comp) {
	e.mu.Lock()
	e.comps[name] = comp
	e.mu.Unlock()
}

type comp struct {
	value     Component
	isDeleted bool
	seq       uint64 // needed for a future version
}

type System interface {
	Update(w *World, dt float64)
}


type World struct {
	nextID          atomic.Uint64
	frame           uint64
	entities        map[Entity]map[string]*comp
	deletedEntities map[Entity]bool
	dirtyComponents map[Entity]map[string]bool
}

func NewWorld() *World {
	return &World{
		entities:        make(map[Entity]map[string]*comp),
		deletedEntities: make(map[Entity]bool),
		dirtyComponents: make(map[Entity]map[string]bool),
	}
}

// CreateEntity creates a new entity
func (w *World) CreateEntity() Entity {
	e := Entity(w.nextID.Add(1) - 1)
	w.entities[e] = make(map[string]*comp)
	return e
}

// DestroyEntity removes an entity and all its components
func (w *World) DestroyEntity(e Entity) {
	delete(w.entities, e)
	w.deletedEntities[e] = true
}

// GetComponent retrieves a component from an entity
func (w *World) GetComponent(e Entity, name string) (Component, bool) {
	if _, exists := w.entities[e]; !exists {
		return nil, false
	}
	c, exists := w.entities[e][name]
	if !exists || c.isDeleted {
		return nil, false
	}
	return c.value, true
}

// HasComponent checks if an entity has a component
func (w *World) HasComponent(e Entity, name string) bool {
	comps, exists := w.entities[e]
	if !exists {
		return false
	}
	c, exists := comps[name]
	return exists && !c.isDeleted
}

// SetComponent sets a component of an entity.
func (w *World) SetComponent(e Entity, name string, val Component) bool {
	if _, exists := w.entities[e]; !exists {
		return false
	}
	if c, exists := w.entities[e][name]; exists {
		c.value = val
		c.isDeleted = false
	} else {
		w.entities[e][name] = &comp{
			value: val,
		}
	}
	if _, ok := w.dirtyComponents[e]; !ok {
		w.dirtyComponents[e] = make(map[string]bool)
	}
	w.dirtyComponents[e][name] = true
	return true
}

// RemoveComponent removes a component from an entity
func (w *World) RemoveComponent(e Entity, name string) bool {
	if _, exists := w.entities[e]; !exists {
		return false
	}
	if _, exists := w.entities[e][name]; !exists {
		return false
	}
	w.entities[e][name].isDeleted = true
	if _, ok := w.dirtyComponents[e]; !ok {
		w.dirtyComponents[e] = make(map[string]bool)
	}
	w.dirtyComponents[e][name] = true
	return true
}

// Query returns all entities that have the specified components
func (w *World) Query(componentNames ...string) []Entity {
	var result []Entity
	for e := range w.entities {
		hasAll := true
		for _, name := range componentNames {
			if !w.HasComponent(e, name) {
				hasAll = false
				break
			}
		}
		if hasAll {
			result = append(result, e)
		}
	}
	return result
}
