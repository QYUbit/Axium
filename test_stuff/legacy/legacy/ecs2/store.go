package main

import (
	"reflect"
	"slices"
	"sync"
	"unsafe"
)

type (
	EntityID    uint32
	ComponentID uint16
	ArchetypeID uint64
)

type Chunk struct {
	data     []byte
	capacity int
	count    int
	pointers [][]unsafe.Pointer
}

func (c *Chunk) set(row, rowSize, off, datSize int, data []byte) bool {
	if row < 0 || row >= c.count {
		return false
	}
	if len(data) < datSize {
		return false
	}

	dst := c.data[row*rowSize+off : row*rowSize+off+datSize]
	copy(dst, data[:datSize])
	return true
}

func (c *Chunk) setRow(row, rowSize int, data []byte) bool {
	if row < 0 || row >= c.count {
		return false
	}
	if len(data) < rowSize {
		return false
	}

	dst := c.data[row*rowSize : row*rowSize+len(data)]
	copy(dst, data[:row])
	return true
}

func (c *Chunk) deleteRow(row, rowSize int) {
	lastRow := c.count - 1

	if lastRow != row {
		srcOffset := lastRow * rowSize
		dstOffset := row * rowSize
		copy(c.data[dstOffset:dstOffset+rowSize], c.data[srcOffset:srcOffset+rowSize])
	}

	c.count--
}

type Archetype struct {
	id           ArchetypeID
	entities     []EntityID
	components   []ComponentID
	offsets      map[ComponentID]int
	componentIdx map[ComponentID]uint8
	chunks       []*Chunk
	rowSize      int
	chunkSize    int
}

func (a *Archetype) getFreeChunk() (*Chunk, int) {
	var chunk *Chunk
	var chunkIdx int

	for i, c := range a.chunks {
		if c.count < c.capacity {
			chunk = c
			chunkIdx = i
			break
		}
	}

	// No free chunk available, create new one
	if chunk == nil {
		chunk = &Chunk{data: make([]byte, a.chunkSize*a.rowSize)}
		chunkIdx = len(a.chunks)
		a.chunks = append(a.chunks, chunk)
	}

	return chunk, chunkIdx
}

func (a *Archetype) insertEntity(e *EntityInfo, comps map[ComponentID][]byte) {
	e.archetype = a.id

	chu, idx := a.getFreeChunk()
	e.chunk = idx
	e.row = chu.count

	data := make([]byte, 0, a.rowSize)

	// a.components are ordered
	for _, cid := range a.components {
		rawComp, ok := comps[cid]
		if !ok || rawComp == nil {
			rawComp = make([]byte, a.offsets[cid])
		}
		data = append(data, rawComp...)
	}

	chu.setRow(chu.count, a.rowSize, data)
	chu.count++
}

func (a *Archetype) deleteEntity(eid EntityID, e *EntityInfo) bool {
	chu := a.chunks[e.chunk]

	e.archetype = 0
	e.chunk = -1
	e.row = -1

	chu.deleteRow(e.row, a.rowSize)

	return true
}

type ArchetypeView struct {
	data     [][][]unsafe.Pointer
	index    map[ComponentID]uint8
	entities []EntityID
}

func (a *Archetype) takeSnapshot() ArchetypeView {
	view := ArchetypeView{
		entities: a.entities,
		index:    a.componentIdx,
		data:     make([][][]unsafe.Pointer, len(a.chunks)),
	}

	for _, c := range a.chunks {
		view.data = append(view.data, c.pointers)
	}
	return view
}

func (a *Archetype) applyArchetypeChanges(changes []Change) {
	var c ComponentInfo

	for _, change := range changes {
		e := change.entity
		chu := a.chunks[e.chunk]
		chu.set(e.row, a.rowSize, a.offsets[change.ComponentId], int(c.size), change.data)
	}
}

type EntityInfo struct {
	archetype ArchetypeID
	chunk     int
	row       int
}

type ComponentInfo struct {
	typ   reflect.Type
	size  uintptr
	align uintptr
}

type Store struct {
	components map[ComponentID]*ComponentInfo
	entities   map[EntityID]*EntityInfo
	archetypes map[ArchetypeID]*Archetype
}

func newStore() *Store {
	return &Store{
		components: make(map[ComponentID]*ComponentInfo),
		entities:   make(map[EntityID]*EntityInfo),
		archetypes: make(map[ArchetypeID]*Archetype),
	}
}

func generateArchetypeID(cids []ComponentID) ArchetypeID {
	id := uint64(14695981039346656037)
	for _, cid := range cids {
		id ^= uint64(cid)
		id *= 1099511628211
	}
	return ArchetypeID(id)
}

func (s *Store) createArchetype(aid ArchetypeID, cids []ComponentID) *Archetype {
	bitIndex := make(map[ComponentID]uint8)
	offsets := make(map[ComponentID]int)
	totalOff := 0

	for i, cid := range cids {
		c, ok := s.components[cid]
		if !ok {
			continue
		}
		aligned := (totalOff + int(c.align) - 1) & ^(int(c.align) - 1)
		offsets[cid] = aligned
		totalOff = aligned + int(c.size)
		bitIndex[cid] = uint8(i)
	}

	a := &Archetype{
		id:           aid,
		components:   cids,
		offsets:      offsets,
		componentIdx: bitIndex,
		rowSize:      totalOff,
		chunkSize:    0,
		chunks:       make([]*Chunk, 0),
		entities:     make([]EntityID, 0, 256),
	}

	s.archetypes[aid] = a
	return a
}

func (s *Store) getOrCreateArchetype(cids []ComponentID) *Archetype {
	sorted := make([]ComponentID, len(cids))
	copy(sorted, cids)
	slices.Sort(sorted)

	aid := generateArchetypeID(sorted)
	a, ok := s.archetypes[aid]
	if ok {
		return a
	}
	return s.createArchetype(aid, sorted)
}

func (s *Store) createEntity(eid EntityID, comps map[ComponentID][]byte) {
	e := &EntityInfo{
		archetype: 0, // quasi invalid value
		chunk:     -1,
		row:       -1,
	}
	s.entities[eid] = e

	cids := make([]ComponentID, len(comps))
	for cid := range comps {
		cids = append(cids, cid)
	}

	a := s.getOrCreateArchetype(cids)
	a.insertEntity(e, comps)
}

func (s *Store) destroyEntity(eid EntityID) bool {

	return true
}

type WorldView struct {
	entities   map[EntityID]*EntityInfo
	archetypes map[ArchetypeID]ArchetypeView
}

func newWorldView() *WorldView {
	return &WorldView{
		entities:   make(map[EntityID]*EntityInfo),
		archetypes: make(map[ArchetypeID]ArchetypeView),
	}
}

func (s *Store) TakeSnapshot() *WorldView {
	view := newWorldView()

	view.entities = s.entities

	for aid, a := range s.archetypes {
		arcSnapShot := a.takeSnapshot()
		view.archetypes[aid] = arcSnapShot
	}

	return view
}

type ChangeType uint8

const (
	CreateEntity ChangeType = iota
	DestroyEntity
	SetComponent
)

type Change struct {
	Type          ChangeType
	EntityId      EntityID
	ComponentId   ComponentID
	Components    map[ComponentID]any
	RawComponents map[ComponentID][]byte
	Value         any
	data          []byte
	entity        *EntityInfo
}

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

func (buf *ChangeBuffer) clear() {
	buf.changes = buf.changes[:0]
}

func (s *Store) prepareChanges(buf *ChangeBuffer) SortedChanges {
	sc := SortedChanges{archtypes: make(map[ArchetypeID][]Change)}

	for _, change := range buf.changes {
		if change.Type != SetComponent {
			sc.structural = append(sc.structural, change)
			continue
		}

		e, ok := s.entities[change.EntityId]
		if !ok {
			continue
		}

		change.entity = e

		a, ok := sc.archtypes[e.archetype]
		if !ok {
			sc.archtypes[e.archetype] = []Change{change}
			continue
		}
		sc.archtypes[e.archetype] = append(a, change)
	}

	return sc
}

type SortedChanges struct {
	structural []Change
	archtypes  map[ArchetypeID][]Change
}

func (s *Store) ProcessChanges(buf *ChangeBuffer) {
	sc := s.prepareChanges(buf)

	for _, change := range sc.structural {
		s.applyStructuralChange(change)
	}

	var wg sync.WaitGroup

	for aid, a := range s.archetypes {
		go func() {
			defer wg.Done()

			changes, ok := sc.archtypes[aid]
			if !ok {
				return
			}

			a.applyArchetypeChanges(changes)
		}()
	}

	wg.Wait()
}

func (s *Store) applyStructuralChange(change Change) {
	switch change.Type {
	case CreateEntity:
		s.createEntity(change.EntityId, change.RawComponents)
	case DestroyEntity:
		s.destroyEntity(change.EntityId)
	}
}
