package main

import (
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"unsafe"
)

type mask128 struct {
	Lo uint64
	Hi uint64
}

func (m *mask128) set(id uint8) {
	if id >= 128 {
		return
	}
	if id < 64 {
		m.Lo |= 1 << id
	} else {
		m.Hi |= 1 << (id - 64)
	}
}

func (m *mask128) has(id uint8) bool {
	if id >= 128 {
		return false
	}
	if id < 64 {
		return m.Lo&(1<<id) != 0
	}
	return m.Hi&(1<<(id-64)) != 0
}

func (m *mask128) clear() {
	m.Lo = 0
	m.Hi = 0
}

type EntityID uint32
type ComponentID uint16
type ArchetypeID uint64

type Entity struct {
	archetype         *Archetype
	chunk             int
	row               int
	isDirty           atomic.Bool
	dirty             mask128
	dirtyMu           sync.Mutex
	addedComponents   []ComponentID
	removedComponents []ComponentID
}

type Component struct {
	id    ComponentID
	typ   reflect.Type
	size  uintptr
	align uintptr
}

type Archetype struct {
	id            ArchetypeID
	entities      []EntityID
	components    []ComponentID
	offsets       map[ComponentID]int
	componentIdx  map[ComponentID]uint8
	rowSize       int
	chunks        []*Chunk
	chunkCapacity int
	rowCapacity   int
	mu            sync.RWMutex
}

type Chunk struct {
	data     []byte
	capacity int
	count    atomic.Int32
	mu       sync.RWMutex
}

type StoreConfig struct {
	CleanupThreshold int
	ChunkCapacity    int
	RowCapacity      int
}

type Store struct {
	archetypes        sync.Map // ArchetypeID -> *Archetype
	components        sync.Map // ComponentID -> Component
	entities          sync.Map // EntityID -> *Entity
	nextEntity        atomic.Uint32
	nextComponent     atomic.Uint32
	createdEntities   []EntityID
	destroyedEntities []EntityID
	entitiesMu        sync.Mutex

	// Buffer pools
	chunkPool          sync.Pool
	entitySlicePool    sync.Pool
	componentSlicePool sync.Pool

	lastCleanupCount atomic.Int32

	cleanupThreshold int
	chunkCapacity    int
	rowCapacity      int
}

func NewStore(config *StoreConfig) *Store {
	var s *Store

	if config == nil {
		s = &Store{
			cleanupThreshold: 1000,
			rowCapacity:      265,
			chunkCapacity:    265,
		}
	} else {
		s = &Store{
			cleanupThreshold: config.CleanupThreshold,
			rowCapacity:      config.RowCapacity,
			chunkCapacity:    config.ChunkCapacity,
		}
	}

	// Initialize pools
	s.chunkPool = sync.Pool{
		New: func() any {
			return &Chunk{
				data:     make([]byte, 0),
				capacity: 256,
			}
		},
	}

	s.entitySlicePool = sync.Pool{
		New: func() any {
			slice := make([]EntityID, 0, 256)
			return &slice
		},
	}

	s.componentSlicePool = sync.Pool{
		New: func() any {
			slice := make([]ComponentID, 0, 16)
			return &slice
		},
	}

	return s
}

func (s *Store) SetCleanupThreshold(threshold int) {
	s.cleanupThreshold = threshold
}

func RegisterComponent[T any](s *Store) ComponentID {
	var zero T
	t := reflect.TypeOf(zero)

	c := Component{
		id:    ComponentID(s.nextComponent.Add(1)),
		typ:   t,
		size:  t.Size(),
		align: uintptr(t.Align()),
	}

	s.components.Store(c.id, c)
	return c.id
}

func (s *Store) ComponentInfo(id ComponentID) (Component, bool) {
	val, ok := s.components.Load(id)
	if !ok {
		return Component{}, false
	}
	return val.(Component), true
}

func (s *Store) GetRaw(eid EntityID, cid ComponentID) (unsafe.Pointer, bool) {
	val, ok := s.entities.Load(eid)
	if !ok {
		return nil, false
	}
	e := val.(*Entity)

	a := e.archetype
	if a == nil {
		return nil, false
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	off, ok := a.offsets[cid]
	if !ok {
		return nil, false
	}

	if e.chunk < 0 || e.chunk >= len(a.chunks) {
		return nil, false
	}

	chu := a.chunks[e.chunk]
	chu.mu.RLock()
	defer chu.mu.RUnlock()

	count := int(chu.count.Load())
	if e.row < 0 || e.row >= count {
		return nil, false
	}

	ptr := unsafe.Pointer(&chu.data[e.row*a.rowSize+off])
	return ptr, true
}

func Get[T any](s *Store, eid EntityID, cid ComponentID) (*T, bool) {
	ptr, ok := s.GetRaw(eid, cid)
	if !ok {
		return nil, false
	}
	return (*T)(ptr), true
}

func (s *Store) SetRaw(eid EntityID, cid ComponentID, src unsafe.Pointer) bool {
	c, ok := s.ComponentInfo(cid)
	if !ok {
		return false
	}

	val, ok := s.entities.Load(eid)
	if !ok {
		return false
	}
	e := val.(*Entity)

	a := e.archetype
	if a == nil {
		return false
	}

	a.mu.RLock()
	off, ok := a.offsets[cid]
	if !ok {
		a.mu.RUnlock()
		return false
	}

	if e.chunk < 0 || e.chunk >= len(a.chunks) {
		a.mu.RUnlock()
		return false
	}

	chu := a.chunks[e.chunk]
	count := int(chu.count.Load())
	if e.row < 0 || e.row >= count {
		a.mu.RUnlock()
		return false
	}

	bitIdx, hasIdx := a.componentIdx[cid]
	a.mu.RUnlock()

	chu.mu.Lock()
	dst := unsafe.Pointer(&chu.data[e.row*a.rowSize+off])
	mem := unsafe.Slice((*byte)(dst), c.size)
	srcBytes := unsafe.Slice((*byte)(src), c.size)
	copy(mem, srcBytes)
	chu.mu.Unlock()

	if hasIdx && bitIdx < 128 {
		e.dirtyMu.Lock()
		e.isDirty.Store(true)
		e.dirty.set(bitIdx)
		e.dirtyMu.Unlock()
	}

	return true
}

func Set[T any](s *Store, eid EntityID, cid ComponentID, value *T) bool {
	return s.SetRaw(eid, cid, unsafe.Pointer(value))
}

func (s *Store) getOrCreateArchetype(components []ComponentID) *Archetype {
	if len(components) >= 128 {
		return nil
	}

	sorted := make([]ComponentID, len(components))
	copy(sorted, components)
	slices.Sort(sorted)

	// FNV-1a hash
	id := uint64(14695981039346656037)
	for _, cid := range sorted {
		id ^= uint64(cid)
		id *= 1099511628211
	}

	if val, ok := s.archetypes.Load(ArchetypeID(id)); ok {
		return val.(*Archetype)
	}

	bitIndex := make(map[ComponentID]uint8)
	offsets := make(map[ComponentID]int)
	totalOff := 0

	for i, cid := range sorted {
		c, ok := s.ComponentInfo(cid)
		if !ok {
			continue
		}
		aligned := (totalOff + int(c.align) - 1) & ^(int(c.align) - 1)
		offsets[cid] = aligned
		totalOff = aligned + int(c.size)
		bitIndex[cid] = uint8(i)
	}

	a := &Archetype{
		id:            ArchetypeID(id),
		components:    sorted,
		offsets:       offsets,
		componentIdx:  bitIndex,
		rowSize:       totalOff,
		chunkCapacity: s.chunkCapacity,
		rowCapacity:   s.rowCapacity,
		chunks:        make([]*Chunk, 0),
		entities:      make([]EntityID, 0, 256),
	}

	actual, loaded := s.archetypes.LoadOrStore(ArchetypeID(id), a)
	if loaded {
		return actual.(*Archetype)
	}
	return a
}

func (s *Store) CreateEntity() EntityID {
	return s.CreateEntityWithComponents(nil)
}

func (s *Store) CreateEntityWithComponents(cids []ComponentID) EntityID {
	eid := EntityID(s.nextEntity.Add(1))

	e := &Entity{
		archetype: nil,
		chunk:     -1,
		row:       -1,
	}
	s.entities.Store(eid, e)

	a := s.getOrCreateArchetype(cids)
	s.moveEntity(nil, a, eid)

	s.entitiesMu.Lock()
	s.createdEntities = append(s.createdEntities, eid)
	s.entitiesMu.Unlock()

	return eid
}

func (s *Store) DestroyEntity(eid EntityID) bool {
	val, ok := s.entities.Load(eid)
	if !ok {
		return false
	}
	e := val.(*Entity)

	a := e.archetype
	if a == nil {
		s.entities.Delete(eid)
		return false
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if e.chunk < 0 || e.chunk >= len(a.chunks) {
		return false
	}

	chu := a.chunks[e.chunk]
	chu.mu.Lock()

	count := int(chu.count.Load())
	lastRow := count - 1

	if e.row != lastRow {
		srcOffset := lastRow * a.rowSize
		dstOffset := e.row * a.rowSize
		copy(chu.data[dstOffset:dstOffset+a.rowSize], chu.data[srcOffset:srcOffset+a.rowSize])

		lastEid := a.entities[len(a.entities)-1]
		if lastVal, ok := s.entities.Load(lastEid); ok {
			lastEntity := lastVal.(*Entity)
			lastEntity.row = e.row
		}
	}

	chu.count.Add(-1)
	chu.mu.Unlock()

	a.entities = a.entities[:len(a.entities)-1]
	s.entities.Delete(eid)

	s.entitiesMu.Lock()
	s.destroyedEntities = append(s.destroyedEntities, eid)
	s.entitiesMu.Unlock()

	s.maybeAutoCleanup()

	return true
}

type DestroyEntityEvent struct {
	EntityID EntityID
}

func (s *Store) moveEntity(from, to *Archetype, eid EntityID) bool {
	val, ok := s.entities.Load(eid)
	if !ok {
		return false
	}
	e := val.(*Entity)

	oldChunk := -1
	oldRow := -1
	if from != nil {
		oldChunk = e.chunk
		oldRow = e.row
	}

	to.mu.Lock()
	defer to.mu.Unlock()

	e.archetype = to

	var chunk *Chunk
	var chunkIdx int

	for i, chu := range to.chunks {
		if int(chu.count.Load()) < chu.capacity {
			chunk = chu
			chunkIdx = i
			break
		}
	}

	if chunk == nil {
		if len(to.chunks) >= to.chunkCapacity {
			return false
		}

		chunk = s.getChunkFromPool(to.rowSize, to.rowCapacity)
		chunkIdx = len(to.chunks)
		to.chunks = append(to.chunks, chunk)
	}

	chunk.mu.Lock()
	row := int(chunk.count.Load())
	e.chunk = chunkIdx
	e.row = row
	chunk.count.Add(1)
	chunk.mu.Unlock()

	to.entities = append(to.entities, eid)

	if from != nil {
		from.mu.RLock()
		oldChunkObj := from.chunks[oldChunk]
		oldChunkObj.mu.RLock()

		for _, cid := range from.components {
			if toOff, ok := to.offsets[cid]; ok {
				fromOff := from.offsets[cid]

				if c, ok := s.ComponentInfo(cid); ok {
					srcData := oldChunkObj.data[oldRow*from.rowSize+fromOff : oldRow*from.rowSize+fromOff+int(c.size)]

					chunk.mu.Lock()
					dstData := chunk.data[row*to.rowSize+toOff : row*to.rowSize+toOff+int(c.size)]
					copy(dstData, srcData)
					chunk.mu.Unlock()
				}
			}
		}

		oldChunkObj.mu.RUnlock()
		from.mu.RUnlock()
	}

	return true
}

func (s *Store) AddComponent(eid EntityID, cid ComponentID) bool {
	val, ok := s.entities.Load(eid)
	if !ok {
		return false
	}
	e := val.(*Entity)

	oldArch := e.archetype
	if oldArch == nil {
		return false
	}

	oldArch.mu.RLock()
	newComponents := make([]ComponentID, len(oldArch.components)+1)
	copy(newComponents, oldArch.components)
	newComponents[len(newComponents)-1] = cid
	oldArch.mu.RUnlock()

	newArch := s.getOrCreateArchetype(newComponents)
	e.addedComponents = append(e.addedComponents, cid)

	return s.moveEntity(oldArch, newArch, eid)
}

func (s *Store) RemoveComponent(eid EntityID, cid ComponentID) bool {
	val, ok := s.entities.Load(eid)
	if !ok {
		return false
	}
	e := val.(*Entity)

	oldArch := e.archetype
	if oldArch == nil {
		return false
	}

	oldArch.mu.RLock()
	newComponents := make([]ComponentID, 0, len(oldArch.components)-1)
	for _, c := range oldArch.components {
		if c != cid {
			newComponents = append(newComponents, c)
		}
	}
	oldArch.mu.RUnlock()

	e.removedComponents = append(e.removedComponents, cid)

	newArch := s.getOrCreateArchetype(newComponents)
	return s.moveEntity(oldArch, newArch, eid)
}

func (s *Store) MarkDirty(eid EntityID, cid ComponentID) bool {
	val, ok := s.entities.Load(eid)
	if !ok {
		return false
	}
	e := val.(*Entity)

	a := e.archetype
	if a == nil {
		return false
	}

	a.mu.RLock()
	idx, ok := a.componentIdx[cid]
	a.mu.RUnlock()

	if !ok {
		return false
	}

	e.dirtyMu.Lock()
	e.isDirty.Store(true)
	e.dirty.set(idx)
	e.dirtyMu.Unlock()

	return true
}

func (s *Store) GetDirtyComponents() map[EntityID]map[ComponentID]unsafe.Pointer {
	results := make(map[EntityID]map[ComponentID]unsafe.Pointer)

	s.entities.Range(func(key, value any) bool {
		eid := key.(EntityID)
		e := value.(*Entity)

		if !e.isDirty.Load() {
			return true
		}

		e.dirtyMu.Lock()
		if !e.isDirty.Load() {
			e.dirtyMu.Unlock()
			return true
		}

		e.isDirty.Store(false)
		dirtyMask := e.dirty
		e.dirty.clear()
		e.dirtyMu.Unlock()

		if e.archetype == nil {
			return true
		}

		e.archetype.mu.RLock()
		for cid, idx := range e.archetype.componentIdx {
			if !dirtyMask.has(idx) {
				continue
			}

			e.archetype.mu.RUnlock()
			ptr, ok := s.GetRaw(eid, cid)
			e.archetype.mu.RLock()

			if ok {
				if results[eid] == nil {
					results[eid] = make(map[ComponentID]unsafe.Pointer)
				}
				results[eid][cid] = ptr
			}
		}
		e.archetype.mu.RUnlock()

		return true
	})

	return results
}

type DirtyRecord struct {
	CreatedEntities   []EntityID
	DestroyedEntities []EntityID
}

func (s *Store) GetDirtyMeta() DirtyRecord {
	s.entitiesMu.Lock()
	defer s.entitiesMu.Unlock()

	dr := DirtyRecord{
		CreatedEntities:   s.createdEntities,
		DestroyedEntities: s.destroyedEntities,
	}

	s.createdEntities = s.createdEntities[:0]
	s.destroyedEntities = s.destroyedEntities[:0]

	return dr
}

func (s *Store) EntityCount() int {
	count := 0
	s.entities.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

func (s *Store) HasComponent(eid EntityID, cid ComponentID) bool {
	val, ok := s.entities.Load(eid)
	if !ok {
		return false
	}
	e := val.(*Entity)

	if e.archetype == nil {
		return false
	}

	e.archetype.mu.RLock()
	_, ok = e.archetype.offsets[cid]
	e.archetype.mu.RUnlock()

	return ok
}

// Pool management
func (s *Store) getChunkFromPool(rowSize, capacity int) *Chunk {
	chunk := s.chunkPool.Get().(*Chunk)

	// Resize if needed
	neededSize := rowSize * capacity
	if cap(chunk.data) < neededSize {
		chunk.data = make([]byte, neededSize)
	} else {
		chunk.data = chunk.data[:neededSize]
	}

	chunk.capacity = capacity
	chunk.count.Store(0)
	return chunk
}

func (s *Store) returnChunkToPool(chunk *Chunk) {
	// Clear sensitive data
	for i := range chunk.data {
		chunk.data[i] = 0
	}
	chunk.count.Store(0)
	s.chunkPool.Put(chunk)
}

func (s *Store) getEntitySlice() *[]EntityID {
	slice := s.entitySlicePool.Get().(*[]EntityID)
	*slice = (*slice)[:0]
	return slice
}

func (s *Store) returnEntitySlice(slice *[]EntityID) {
	if cap(*slice) > 4096 {
		return // Don't pool huge slices
	}
	s.entitySlicePool.Put(slice)
}

func (s *Store) getComponentSlice() *[]ComponentID {
	slice := s.componentSlicePool.Get().(*[]ComponentID)
	*slice = (*slice)[:0]
	return slice
}

func (s *Store) returnComponentSlice(slice *[]ComponentID) {
	if cap(*slice) > 256 {
		return
	}
	s.componentSlicePool.Put(slice)
}

// Cleanup methods
func (s *Store) maybeAutoCleanup() {
	current := s.lastCleanupCount.Add(1)
	if int(current) >= s.cleanupThreshold {
		s.lastCleanupCount.Store(0)
		go s.Cleanup(false) // Run in background
	}
}

// Cleanup performs maintenance operations
// aggressive: if true, more thorough but slower cleanup
func (s *Store) Cleanup(aggressive bool) {
	// 1. Remove empty archetypes
	s.cleanupEmptyArchetypes()

	// 2. Compact archetypes with fragmentation
	if aggressive {
		s.compactFragmentedArchetypes()
	}

	// 3. Remove excess chunks
	s.cleanupExcessChunks()
}

func (s *Store) cleanupEmptyArchetypes() {
	toDelete := make([]ArchetypeID, 0)

	s.archetypes.Range(func(key, value any) bool {
		aid := key.(ArchetypeID)
		arch := value.(*Archetype)

		arch.mu.RLock()
		isEmpty := len(arch.entities) == 0
		arch.mu.RUnlock()

		if isEmpty {
			toDelete = append(toDelete, aid)
		}

		return true
	})

	for _, aid := range toDelete {
		if val, ok := s.archetypes.Load(aid); ok {
			arch := val.(*Archetype)

			arch.mu.Lock()
			// Double-check after acquiring lock
			if len(arch.entities) == 0 {
				// Return chunks to pool
				for _, chunk := range arch.chunks {
					s.returnChunkToPool(chunk)
				}
				arch.chunks = nil
				s.archetypes.Delete(aid)
			}
			arch.mu.Unlock()
		}
	}
}

func (s *Store) compactFragmentedArchetypes() {
	s.archetypes.Range(func(_, value any) bool {
		arch := value.(*Archetype)

		arch.mu.Lock()
		defer arch.mu.Unlock()

		if len(arch.chunks) <= 1 {
			return true
		}

		// Calculate fragmentation
		totalCapacity := 0
		totalUsed := 0
		for _, chunk := range arch.chunks {
			totalCapacity += chunk.capacity
			totalUsed += int(chunk.count.Load())
		}

		if totalCapacity == 0 {
			return true
		}

		fragmentation := float64(totalCapacity-totalUsed) / float64(totalCapacity)

		// If more than 50% fragmented, compact
		if fragmentation > 0.5 && totalUsed > 0 {
			s.compactArchetype(arch)
		}

		return true
	})
}

func (s *Store) compactArchetype(arch *Archetype) {
	// Calculate needed chunks
	totalEntities := len(arch.entities)
	if totalEntities == 0 {
		return
	}

	neededChunks := (totalEntities + arch.rowCapacity - 1) / arch.rowCapacity
	if neededChunks >= len(arch.chunks) {
		return // No compaction needed
	}

	// Create new compacted chunks
	newChunks := make([]*Chunk, 0, neededChunks)
	entityIdx := 0

	for i := 0; i < neededChunks; i++ {
		chunk := s.getChunkFromPool(arch.rowSize, arch.rowCapacity)

		for j := 0; j < arch.rowCapacity && entityIdx < totalEntities; j++ {
			eid := arch.entities[entityIdx]

			if val, ok := s.entities.Load(eid); ok {
				e := val.(*Entity)
				oldChunk := arch.chunks[e.chunk]

				// Copy data
				oldChunk.mu.RLock()
				srcOffset := e.row * arch.rowSize
				dstOffset := j * arch.rowSize
				copy(chunk.data[dstOffset:dstOffset+arch.rowSize],
					oldChunk.data[srcOffset:srcOffset+arch.rowSize])
				oldChunk.mu.RUnlock()

				// Update entity location
				e.chunk = i
				e.row = j
			}

			entityIdx++
		}

		chunk.count.Store(int32(entityIdx - i*arch.rowCapacity))
		newChunks = append(newChunks, chunk)
	}

	// Return old chunks to pool
	for _, chunk := range arch.chunks {
		s.returnChunkToPool(chunk)
	}

	arch.chunks = newChunks
}

func (s *Store) cleanupExcessChunks() {
	s.archetypes.Range(func(_, value any) bool {
		arch := value.(*Archetype)

		arch.mu.Lock()
		defer arch.mu.Unlock()

		// Remove trailing empty chunks
		for len(arch.chunks) > 0 {
			lastChunk := arch.chunks[len(arch.chunks)-1]
			if lastChunk.count.Load() == 0 {
				s.returnChunkToPool(lastChunk)
				arch.chunks = arch.chunks[:len(arch.chunks)-1]
			} else {
				break
			}
		}

		return true
	})
}

// Stats for monitoring
type StoreStats struct {
	EntityCount      int
	ArchetypeCount   int
	TotalChunks      int
	EmptyChunks      int
	Fragmentation    float64
	MemoryUsageBytes int64
}

func (s *Store) GetStats() StoreStats {
	stats := StoreStats{}

	s.entities.Range(func(_, _ any) bool {
		stats.EntityCount++
		return true
	})

	totalCapacity := 0
	totalUsed := 0

	s.archetypes.Range(func(_, value any) bool {
		arch := value.(*Archetype)
		stats.ArchetypeCount++

		arch.mu.RLock()
		stats.TotalChunks += len(arch.chunks)

		for _, chunk := range arch.chunks {
			count := int(chunk.count.Load())
			if count == 0 {
				stats.EmptyChunks++
			}
			totalCapacity += chunk.capacity
			totalUsed += count
			stats.MemoryUsageBytes += int64(len(chunk.data))
		}
		arch.mu.RUnlock()

		return true
	})

	if totalCapacity > 0 {
		stats.Fragmentation = float64(totalCapacity-totalUsed) / float64(totalCapacity)
	}

	return stats
}
