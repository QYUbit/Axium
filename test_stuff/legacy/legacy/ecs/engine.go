package main

import (
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// ==================================================================
// Systems
// ==================================================================

type System interface {
	Init(se *StateEngine)
	Update(se *StateEngine, dt time.Duration)
	Name() string
	Stage() int
}

type BaseSystem struct {
	name  string
	stage int
}

func NewBaseSystem(name string, stage int) BaseSystem {
	return BaseSystem{name, stage}
}

func (bs BaseSystem) Name() string { return bs.name }

func (bs BaseSystem) Stage() int { return bs.stage }

func (bs BaseSystem) Init(_ *StateEngine) {}

func (bs BaseSystem) Update(_ *StateEngine, _ time.Duration) {}

// ==================================================================
// Engine
// ==================================================================

type StateEngine struct {
	store  *Store
	events *eventBus

	systems map[int][]System
	stages  []int

	clients map[string]*ClientState

	version atomic.Uint64

	componentNames map[string]ComponentID
}

func NewStateEngine(store *Store, bus *eventBus) *StateEngine {
	return &StateEngine{
		store:          store,
		events:         bus,
		systems:        make(map[int][]System),
		componentNames: make(map[string]ComponentID),
		clients:        make(map[string]*ClientState),
	}
}

// ==================================================================
// Store
// ==================================================================

func RegisterComponent[T any](se *StateEngine, name string) {
	var zero T
	t := reflect.TypeOf(zero)

	cid := se.store.registerComponent(t)
	se.componentNames[name] = cid
}

func AutoRegisterComponent[T any](se *StateEngine) {
	var zero T
	t := reflect.TypeOf(zero)

	name := t.Name()
	pkgPath := t.PkgPath()
	if pkgPath != "main" {
		parts := strings.Split(pkgPath, "/")
		name = parts[len(parts)-1] + "." + name
	}

	cid := se.store.registerComponent(t)
	se.componentNames[name] = cid
}

func (se *StateEngine) GetComponentID(name string) (ComponentID, bool) {
	cid, ok := se.componentNames[name]
	return cid, ok
}

func (se *StateEngine) CreateEntity() EntityID {
	return se.store.CreateEntityWithComponents(nil)
}

func (se *StateEngine) CreateEntityWithComponents(names ...string) EntityID {
	var cids []ComponentID
	for _, name := range names {
		cid, ok := se.componentNames[name]
		if ok {
			cids = append(cids, cid)
		}
	}
	return se.store.CreateEntityWithComponents(cids)
}

func (se *StateEngine) DestroyEntity(eid EntityID) bool {
	return se.store.DestroyEntity(eid)
}

func (se *StateEngine) AddComponent(eid EntityID, cname string) bool {
	cid, ok := se.componentNames[cname]
	if !ok {
		return false
	}
	return se.store.AddComponent(eid, cid)
}

func (se *StateEngine) RemoveComponent(eid EntityID, cname string) bool {
	cid, ok := se.componentNames[cname]
	if !ok {
		return false
	}
	return se.store.RemoveComponent(eid, cid)
}

func Get[T any](se *StateEngine, eid EntityID, cname string) (*T, bool) {
	cid, ok := se.componentNames[cname]
	if !ok {
		return nil, false
	}
	ptr, ok := se.store.GetRaw(eid, cid)
	if !ok {
		return nil, false
	}
	return (*T)(ptr), true
}

func Set[T any](se *StateEngine, eid EntityID, cname string, value *T) bool {
	cid, ok := se.componentNames[cname]
	if !ok {
		return false
	}
	return se.store.SetRaw(eid, cid, unsafe.Pointer(value))
}

func (se *StateEngine) MarkDirty(eid EntityID, cname string) bool {
	cid, ok := se.componentNames[cname]
	if !ok {
		return false
	}
	return se.store.MarkDirty(eid, cid)
}

// ==================================================================
// System Management
// ==================================================================

func (se *StateEngine) RegisterSystem(sys System) {
	inserted := false
	for i, stage := range se.stages {

		if sys.Stage() < stage {
			se.stages = append(se.stages[:i], append([]int{sys.Stage()}, se.stages[i:]...)...)
			inserted = true
			break
		}
	}

	if !inserted {
		se.stages = append(se.stages, sys.Stage())
	}

	systems, ok := se.systems[sys.Stage()]
	if !ok {
		se.systems[sys.Stage()] = []System{sys}
		return
	}

	se.systems[sys.Stage()] = append(systems, sys)
}

func (se *StateEngine) UnregisterSystem(sys System) bool {
	systems, ok := se.systems[sys.Stage()]
	if !ok {
		return false
	}

	deleted := false
	for i, s := range systems {
		if sys.Name() == s.Name() {
			systems = append(systems[:i], systems[i+1:]...)
			deleted = true
			break
		}
	}

	if !deleted {
		return false
	}

	if len(systems) == 0 {
		delete(se.systems, sys.Stage())
		for i, stage := range se.stages {
			if stage == sys.Stage() {
				se.stages = append(se.stages[:i], se.stages[i+1:]...)
				break
			}
		}
		return true
	}

	se.systems[sys.Stage()] = systems
	return true
}

func (se *StateEngine) Update(dt time.Duration) {
	startTime := time.Now()

	var wg sync.WaitGroup
	for _, stage := range se.stages {
		systems, ok := se.systems[stage]
		if !ok {
			continue
		}

		wg.Wait()

		for _, sys := range systems {
			wg.Add(1)
			go func(s System) {
				defer wg.Done()
				s.Update(se, dt)
			}(sys)
		}
	}
	wg.Wait()

	_ = time.Since(startTime)
}

// ==================================================================
// Client Management
// ==================================================================

type ClientState struct {
	lastVersion      uint64
	observedEntities map[EntityID]struct{}
	mu               sync.RWMutex
}

func newClientState() *ClientState {
	return &ClientState{
		lastVersion:      0,
		observedEntities: make(map[EntityID]struct{}),
	}
}

func (cs *ClientState) addObservedEntity(entityId EntityID) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.observedEntities[entityId] = struct{}{}
}

func (cs *ClientState) removeObservedEntity(entityId EntityID) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	delete(cs.observedEntities, entityId)
}

func (cs *ClientState) getObservedEntities() []EntityID {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	result := make([]EntityID, 0, len(cs.observedEntities))
	for entityId := range cs.observedEntities {
		result = append(result, entityId)
	}
	return result
}

func (cs *ClientState) setLastVersion(version uint64) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.lastVersion = version
}

func (cs *ClientState) getLastVersion() uint64 {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.lastVersion
}

func (se *StateEngine) RegisterClient(clientId string) {
	if _, exists := se.clients[clientId]; !exists {
		se.clients[clientId] = newClientState()
	}
}

func (se *StateEngine) UnregisterClient(clientId string) {
	delete(se.clients, clientId)
}

func (se *StateEngine) AddObservedEntity(clientId string, eid EntityID) bool {
	client, exists := se.clients[clientId]

	if !exists {
		return false
	}

	if !se.store.EntityExists(eid) {
		return false
	}

	client.addObservedEntity(eid)
	return true
}

func (se *StateEngine) RemoveObservedEntity(clientId string, eid EntityID) bool {
	client, exists := se.clients[clientId]

	if !exists {
		return false
	}

	if !se.store.EntityExists(eid) {
		return false
	}

	client.removeObservedEntity(eid)
	return true
}

// ==================================================================
// Delta Stuff
// ==================================================================

type HistoricalDelta struct {
	delta   DeltaRecord
	version uint64
}

type DeltaHistory struct {
	deltas    []HistoricalDelta
	mu        sync.RWMutex
	maxDeltas int
}

func (dh *DeltaHistory) add(delta HistoricalDelta) {
	dh.mu.Lock()
	defer dh.mu.Unlock()

	dh.deltas = append(dh.deltas, delta)

	// Truncate oldest delta
	if len(dh.deltas) > dh.maxDeltas {
		dh.deltas = dh.deltas[1:]
	}
}

func (dh *DeltaHistory) getRange(fromVersion, toVersion uint64) ([]HistoricalDelta, bool) {
	dh.mu.RLock()
	defer dh.mu.RUnlock()

	if len(dh.deltas) == 0 {
		return nil, false
	}

	oldestVersion := dh.deltas[0].version

	if fromVersion < oldestVersion {
		return nil, false
	}

	result := make([]HistoricalDelta, 0)
	for _, delta := range dh.deltas {
		if delta.version > fromVersion && delta.version <= toVersion {
			result = append(result, delta)
		}
	}

	return result, true
}
