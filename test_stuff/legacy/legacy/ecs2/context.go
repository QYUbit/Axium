package main

import (
	"sync/atomic"
	"unsafe"
)

type WorldRegistry struct {
	nextEntity atomic.Int32
}

func (r *WorldRegistry) makeEntity() EntityID {
	return EntityID(r.nextEntity.Add(1))
}

type WorldContext struct {
	registry *WorldRegistry
	view     *WorldView
	changes  *ChangeBuffer
}

func newWorldContext(registry *WorldRegistry, view *WorldView, changes *ChangeBuffer) *WorldContext {
	return &WorldContext{
		registry: registry,
		view:     view,
		changes:  changes,
	}
}

func Set[T any](ctx *WorldContext, eid EntityID, cid ComponentID, value T) {
	size := unsafe.Sizeof(value)
	data := make([]byte, size)

	src := unsafe.Pointer(&value)
	dst := unsafe.Slice((*byte)(unsafe.Pointer(&data[0])), size)
	copy(dst, unsafe.Slice((*byte)(src), size))

	c := Change{
		Type:        SetComponent,
		EntityId:    eid,
		ComponentId: cid,
		Value:       value,
		data:        data,
	}
	ctx.changes.appendChange(c)
}

func (ctx *WorldContext) CreateEntity(components ...ComponentID) EntityID {
	eid := ctx.registry.makeEntity()

	rawComps := make(map[ComponentID][]byte, len(components))
	comps := make(map[ComponentID]any, len(components))
	for _, cid := range components {
		rawComps[cid] = nil
		comps[cid] = nil
	}

	c := Change{
		Type:          CreateEntity,
		EntityId:      eid,
		RawComponents: rawComps,
		Components:    comps,
	}
	ctx.changes.appendChange(c)

	return eid
}

type ComponentFactory struct {
	value any
	raw   []byte
}

func NewComponent[T any](v T) ComponentFactory {
	size := int(unsafe.Sizeof(v))
	if size == 0 {
		return ComponentFactory{}
	}
	raw := make([]byte, size)
	src := unsafe.Slice((*byte)(unsafe.Pointer(&v)), size)
	copy(raw, src)
	return ComponentFactory{
		value: v,
		raw:   raw,
	}
}

func (ctx *WorldContext) CreateEntityWithData(components map[ComponentID]ComponentFactory) EntityID {
	eid := ctx.registry.makeEntity()

	rawComps := make(map[ComponentID][]byte, len(components))
	comps := make(map[ComponentID]any, len(components))
	for cid, c := range components {
		rawComps[cid] = c.raw
		comps[cid] = c.value
	}

	c := Change{
		Type:          CreateEntity,
		EntityId:      eid,
		Components:    comps,
		RawComponents: rawComps,
	}
	ctx.changes.appendChange(c)

	return eid
}

func (ctx *WorldContext) DestroyEntity(entity EntityID) {
	c := Change{
		Type:     DestroyEntity,
		EntityId: entity,
	}
	ctx.changes.appendChange(c)
}

func (ctx *WorldContext) CollectChanges() []Change {
	return ctx.changes.getChanges()
}

func (ctx *WorldContext) EntityExists(entity EntityID) bool {
	_, exists := ctx.view.entities[entity]
	return exists
}

func (ctx *WorldContext) HasComponent(entity EntityID, component ComponentID) bool {
	e, ok := ctx.view.entities[entity]
	if !ok {
		return false
	}
	a, ok := ctx.view.archetypes[e.archetype]
	if !ok {
		return false
	}
	_, has := a.index[component]
	return has
}

type Query struct {
	include []ComponentID
	exclude []ComponentID
}

func NewQuery() Query {
	return Query{
		include: make([]ComponentID, 0),
		exclude: make([]ComponentID, 0),
	}
}

func (q Query) With(componentType ...ComponentID) Query {
	q.include = append(q.include, componentType...)
	return q
}

func (q Query) Without(componentType ...ComponentID) Query {
	q.exclude = append(q.exclude, componentType...)
	return q
}

func (ctx *WorldContext) Query(query Query) []EntityID {
	result := []EntityID{}

	aid := generateArchetypeID(query.include)

	a, ok := ctx.view.archetypes[aid]
	if !ok {
		return result
	}

	// Ignore excluded

	result = append(result, a.entities...)
	return result
}

func GetComponent[T any](ctx *WorldContext, entity EntityID, component ComponentID) (T, bool) {
	var zero T
	e, ok := ctx.view.entities[entity]
	if !ok {
		return zero, false
	}
	a, ok := ctx.view.archetypes[e.archetype]
	if !ok {
		return zero, false
	}
	ptr := a.data[e.chunk][e.row][a.index[component]]
	return *(*T)(ptr), true
}
