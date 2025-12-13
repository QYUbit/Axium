package ecs

import (
	"iter"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"unsafe"
)

// ==================================================================
// Types
// ==================================================================

type (
	EntityID    uint32
	ComponentID uint16
	ArchetypeID uint64
)

type System func(w *World, dt float64)

type Entity struct {
	id        EntityID
	archetype *Archetype
	row       int
}

type ComponentInfo struct {
	id    ComponentID
	typ   reflect.Type
	size  uintptr
	align uintptr
}

type ChangeType uint8

const (
	SetComponent ChangeType = iota
	CreateEntity
	DestroyEntity
)

type Change struct {
	Type          ChangeType
	EntityId      EntityID
	ComponentId   ComponentID
	rawComponents map[ComponentID][]byte
	data          []byte
	entity        *Entity
}

type StateType uint8

const (
	RegistrationPhase StateType = iota
	SystemsPhase
	ManagedPhase
)

type ComponentFactory struct {
	raw []byte
	val any
	typ reflect.Type
}

// ==================================================================
// Archetype
// ==================================================================

type Archetype struct {
	id         ArchetypeID
	entities   []EntityID
	components []ComponentID
	count      int
	data       map[ComponentID][]byte
	sizes      map[ComponentID]int
}

type ArchetypeView struct {
	archetype *Archetype
}

func (a *Archetype) getComponents(cid ComponentID) ([]byte, bool) {
	data, ok := a.data[cid]
	return data, ok
}

func (a *Archetype) setComponent(e *Entity, cid ComponentID, dat []byte) {
	size, ok := a.sizes[cid]
	if !ok {
		return
	}
	if len(dat) != size {
		return
	}

	data, ok := a.data[cid]
	if !ok {
		return
	}
	if len(data) < (e.row+1)*size {
		return
	}

	dst := data[e.row*size : e.row*size+size]
	copy(dst, dat[:size])
}

func (a *Archetype) insertEntity(e *Entity, comps map[ComponentID][]byte) {
	for _, cid := range a.components {
		size := a.sizes[cid]

		rawComp, ok := comps[cid]
		if !ok || rawComp == nil {
			rawComp = make([]byte, size)
		} else if len(rawComp) != size {
			tmp := make([]byte, size)
			copy(tmp, rawComp)
			rawComp = tmp
		}

		a.data[cid] = append(a.data[cid], rawComp...)
	}

	e.row = a.count
	e.archetype = a

	a.entities = append(a.entities, e.id)
	a.count++
}

func (a *Archetype) deleteEntity(e *Entity) {
	idx := slices.Index(a.entities, e.id)
	if idx == -1 {
		return
	}
	a.entities = append(a.entities[:idx], a.entities[idx+1:]...)

	last := a.count - 1
	a.count--

	// Complicated

}

func (a *Archetype) applyChanges(changes []Change) {
	for _, change := range changes {
		a.setComponent(change.entity, change.ComponentId, change.data)
	}
}

// ==================================================================
// World
// ==================================================================

type World struct {
	entities       map[EntityID]*Entity
	components     map[ComponentID]*ComponentInfo
	componentTypes map[reflect.Type]*ComponentInfo
	archetypes     map[ArchetypeID]*Archetype

	changeBuf *ChangeBuffer

	systems []System

	started atomic.Bool
	state   StateType
}

func NewWorld() *World {
	return &World{
		entities:       make(map[EntityID]*Entity),
		components:     make(map[ComponentID]*ComponentInfo),
		componentTypes: make(map[reflect.Type]*ComponentInfo),
		archetypes:     make(map[ArchetypeID]*Archetype),

		changeBuf: &ChangeBuffer{},

		state: RegistrationPhase,
	}
}

// ==================================================================
// Registration
// ==================================================================

func (w *World) RegisterSystem(system System) {
	w.systems = append(w.systems, system)
}

func RegisterComponent[T any](w *World, id ComponentID) {
	var zero T
	t := reflect.TypeOf(zero)

	c := &ComponentInfo{
		id:    id,
		typ:   t,
		size:  t.Size(),
		align: uintptr(t.Align()),
	}

	w.components[id] = c
	w.componentTypes[t] = c
}

// ==================================================================
// Internal Operations
// ==================================================================

func generateArchetypeID(cids []ComponentID) ArchetypeID {
	id := uint64(14695981039346656037)
	for _, cid := range cids {
		id ^= uint64(cid)
		id *= 1099511628211
	}
	return ArchetypeID(id)
}

func (w *World) createArchetype(aid ArchetypeID, cids []ComponentID) *Archetype {
	if len(cids) > 127 {
		return nil
	}

	compIds := make([]ComponentID, 0, len(cids))
	sizes := make(map[ComponentID]int, len(cids))

	for _, cid := range cids {
		c, ok := w.components[cid]
		if !ok {
			continue
		}
		compIds = append(compIds, cid)
		sizes[cid] = int(c.size)
	}

	a := &Archetype{
		id:         aid,
		components: compIds,
		entities:   make([]EntityID, 0, 256),
		data:       make(map[ComponentID][]byte),
		sizes:      sizes,
	}

	w.archetypes[aid] = a
	return a
}

func (w *World) getOrCreateArchetype(cids []ComponentID) *Archetype {
	sorted := make([]ComponentID, len(cids))
	copy(sorted, cids)
	slices.Sort(sorted)

	aid := generateArchetypeID(sorted)
	a, ok := w.archetypes[aid]
	if ok {
		return a
	}
	return w.createArchetype(aid, sorted) // ! Can return nil
}

func (w *World) createEntity(eid EntityID, initial map[ComponentID][]byte) {
	cids := make([]ComponentID, len(initial))
	for cid := range initial {
		cids = append(cids, cid)
	}

	a := w.getOrCreateArchetype(cids)
	if a == nil {
		return
	}

	e := &Entity{
		id:        eid,
		archetype: nil,
		row:       -1,
	}

	a.insertEntity(e, initial)

	w.entities[eid] = e
}

func (w *World) destroyEntity(eid EntityID) {
	e, ok := w.entities[eid]
	if !ok {
		return
	}

	e.archetype.deleteEntity(e)

	delete(w.entities, eid)
}

// ==================================================================
// Querying
// ==================================================================

type Query struct {
	required []ComponentID
	excluded []ComponentID
}

func NewQuery() *Query {
	return &Query{}
}

func (q *Query) With(components ...ComponentID) *Query {
	q.required = append(q.required, components...)
	return q
}

func (q *Query) Without(components ...ComponentID) *Query {
	q.excluded = append(q.required, components...)
	return q
}

func (q *Query) matches(a *Archetype) bool {
	matching := true
	for _, cid := range q.required {
		if _, ok := a.data[cid]; !ok {
			matching = false
			break
		}
	}

	if !matching {
		return false
	}

	for _, cid := range q.excluded {
		if _, ok := a.data[cid]; ok {
			matching = false
			break
		}
	}

	return matching
}

func (q *Query) Execute(w *World) iter.Seq[*ArchetypeView] {
	return func(yield func(*ArchetypeView) bool) {
		for _, a := range w.archetypes {
			if !q.matches(a) {
				continue
			}

			view := &ArchetypeView{
				archetype: a,
			}

			if !yield(view) {
				return
			}
		}
	}
}

// ==================================================================
// Read
// ==================================================================

func castSlice[T any](data []byte) []T {
	var zero T
	size := unsafe.Sizeof(zero)

	count := len(data) / int(size)
	ptr := unsafe.Pointer(&data[0])
	return unsafe.Slice((*T)(ptr), count)
}

func GetSlice[T any](v *ArchetypeView, cid ComponentID) []T {
	comps, ok := v.archetype.getComponents(cid)
	if !ok {
		return []T{}
	}
	return castSlice[T](comps)
}

func (v *ArchetypeView) GetEntities() []EntityID {
	return v.archetype.entities
}

func (w *World) EntityExists(entity EntityID) bool {
	_, ok := w.entities[entity]
	return ok
}

func (w *World) HasComponent(entity EntityID, component ComponentID) bool {
	e, ok := w.entities[entity]
	if !ok {
		return false
	}

	_, ok = e.archetype.data[component]
	return ok
}

// ==================================================================
// Updates
// ==================================================================

type SortedChanges struct {
	structural []Change
	archtypes  map[ArchetypeID][]Change
}

func (w *World) prepareChanges() SortedChanges {
	sc := SortedChanges{archtypes: make(map[ArchetypeID][]Change)}

	for _, change := range w.changeBuf.getChanges() {
		if change.Type != SetComponent {
			sc.structural = append(sc.structural, change)
			continue
		}

		e, ok := w.entities[change.EntityId]
		if !ok {
			continue
		}

		change.entity = e

		a, ok := sc.archtypes[e.archetype.id]
		if !ok {
			sc.archtypes[e.archetype.id] = []Change{change}
			continue
		}
		sc.archtypes[e.archetype.id] = append(a, change)
	}

	return sc
}

func (w *World) Update(dt float64) {
	var wg sync.WaitGroup
	for _, system := range w.systems {
		wg.Add(1)
		go func() {
			defer wg.Done()
			system(w, dt)
		}()
	}
	wg.Wait()

	sc := w.prepareChanges()

	for _, change := range sc.structural {
		switch change.Type {
		case CreateEntity:
			w.createEntity(change.EntityId, change.rawComponents)
		case DestroyEntity:
			w.destroyEntity(change.EntityId)
		}
	}

	for aid, a := range w.archetypes {
		wg.Add(1)
		go func() {
			defer wg.Done()

			changes, ok := sc.archtypes[aid]
			if !ok {
				return
			}

			a.applyChanges(changes)
		}()
	}

	wg.Wait()

	w.changeBuf.changes = w.changeBuf.changes[:0]
}

// ==================================================================
// Changes
// ==================================================================

type ChangeBuffer struct {
	changes []Change
	mu      sync.Mutex
}

func (buf *ChangeBuffer) appendChange(c Change) {
	buf.mu.Lock()
	buf.changes = append(buf.changes, c)
	buf.mu.Unlock()
}

func (buf *ChangeBuffer) getChanges() []Change {
	buf.mu.Lock()
	defer buf.mu.Unlock()
	return buf.changes
}

func NewComponent[T any](v T) ComponentFactory {
	size := unsafe.Sizeof(v)
	data := make([]byte, size)

	src := unsafe.Pointer(&v)
	dst := unsafe.Slice((*byte)(unsafe.Pointer(&data[0])), size)
	copy(dst, unsafe.Slice((*byte)(src), size))

	var zero T
	typ := reflect.TypeOf(zero)

	return ComponentFactory{
		raw: data,
		val: v,
		typ: typ,
	}
}

func (w *World) CreateEntity(id EntityID, initial ...ComponentFactory) {
	components := make(map[ComponentID][]byte, len(initial))
	for _, comp := range initial {
		c, ok := w.componentTypes[comp.typ]
		if !ok {
			continue
		}
		components[c.id] = comp.raw
	}

	w.changeBuf.appendChange(Change{
		Type:          CreateEntity,
		EntityId:      id,
		rawComponents: components,
	})
}

func (w *World) DestroyEntity(id EntityID) {
	w.changeBuf.appendChange(Change{
		Type:     DestroyEntity,
		EntityId: id,
	})
}

func (w *World) SetByID(entity EntityID, component ComponentID, data ComponentFactory) {
	w.changeBuf.appendChange(Change{
		Type:        SetComponent,
		EntityId:    entity,
		ComponentId: component,
		data:        data.raw,
	})
}

func (w *World) Set(entity EntityID, data ComponentFactory) {
	c, ok := w.componentTypes[data.typ]
	if !ok {
		return
	}

	w.changeBuf.appendChange(Change{
		Type:        SetComponent,
		EntityId:    entity,
		ComponentId: c.id,
		data:        data.raw,
	})
}

// ==================================================================
// Example
// ==================================================================

const (
	PositionType ComponentID = iota
	VelocityType
)

type Position struct{ X, Y float64 }
type Velocity struct{ X, Y float64 }

func MovementSystem(w *World, dt float64) {
	query := NewQuery().With(PositionType, VelocityType)

	for view := range query.Execute(w) {
		positions := GetSlice[Position](view, PositionType)
		velocities := GetSlice[Velocity](view, VelocityType)
		entities := view.GetEntities()

		for i := range entities {
			pos := Position{
				X: positions[i].X + velocities[i].X,
				Y: positions[i].Y + velocities[i].Y,
			}
			w.Set(entities[i], NewComponent(pos))
		}
	}
}

func main() {
	w := NewWorld()

	RegisterComponent[Position](w, PositionType)
	RegisterComponent[Velocity](w, VelocityType)

	player := EntityID(0)
	w.CreateEntity(
		player,
		NewComponent(Position{}),
		NewComponent(Velocity{}),
	)

	w.RegisterSystem(MovementSystem)
}
