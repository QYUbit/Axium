package ecs

import (
	"fmt"
	"maps"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// Core Types
// ============================================================================

type EntityId uint64

type ComponentType string

type ClientId string

type Serializer interface {
	Serialize() ([]byte, error)
}

type Component interface {
	Serializer
	Type() string
}

type System interface {
	Update(dt time.Duration, entities []EntityId, world *World)
	Components() []ComponentType
	Priority() int
}

// ============================================================================
// Delta Types
// ============================================================================

type DeltaType uint8

const (
	DeltaEntityCreated DeltaType = iota
	DeltaEntityDestroyed
	DeltaComponentAdded
	DeltaComponentUpdated
	DeltaComponentRemoved
)

type Delta struct {
	Type      DeltaType
	EntityId  EntityId
	Component Component
}

type ClientDelta struct {
	BaseVersion    uint64
	NewVersion     uint64
	Deltas         []Delta
	ResyncRequired bool
}

// ============================================================================
// Errors
// ============================================================================

type ErrClientNotFound struct {
	ClinetId ClientId
}

func (e ErrClientNotFound) Error() string {
	return fmt.Sprintf("client %s not registered", e.ClinetId)
}

type ErrEntityNotFound struct {
	EntityId EntityId
}

func (e ErrEntityNotFound) Error() string {
	return fmt.Sprintf("component %d does not exist", e.EntityId)
}

type ErrComponentTypeNotFound struct {
	ComponentType ComponentType
}

func (e ErrComponentTypeNotFound) Error() string {
	return fmt.Sprintf("component type %s not found", e.ComponentType)
}

type ErrComponentNotFound struct {
	EntityId      EntityId
	ComponentType ComponentType
}

func (e ErrComponentNotFound) Error() string {
	return fmt.Sprintf("component %s not found for entity %d", e.ComponentType, e.EntityId)
}

// ============================================================================
// Component Store
// ============================================================================

type componentStore struct {
	data  map[EntityId]Component
	dirty map[EntityId]struct{}
	mu    sync.RWMutex
}

func newComponentStore() *componentStore {
	return &componentStore{
		data:  make(map[EntityId]Component),
		dirty: make(map[EntityId]struct{}),
	}
}

func (cs *componentStore) set(entityId EntityId, component Component) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.data[entityId] = component
	cs.dirty[entityId] = struct{}{}
}

func (cs *componentStore) get(entityId EntityId) (Component, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	comp, ok := cs.data[entityId]
	return comp, ok
}

func (cs *componentStore) remove(entityId EntityId) bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	_, existed := cs.data[entityId]
	delete(cs.data, entityId)
	cs.dirty[entityId] = struct{}{}
	return existed
}

func (cs *componentStore) getDirty() map[EntityId]Component {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	result := make(map[EntityId]Component, len(cs.dirty))
	for entityId := range cs.dirty {
		if comp, ok := cs.data[entityId]; ok {
			result[entityId] = comp
		}
	}
	return result
}

func (cs *componentStore) clearDirty() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.dirty = make(map[EntityId]struct{})
}

func (cs *componentStore) getAll() map[EntityId]Component {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	result := make(map[EntityId]Component, len(cs.data))
	maps.Copy(result, cs.data)
	return result
}

// ============================================================================
// Client State
// ============================================================================

type ClientState struct {
	lastVersion      uint64
	observedEntities map[EntityId]struct{}
	mu               sync.RWMutex
}

func newClientState() *ClientState {
	return &ClientState{
		lastVersion:      0,
		observedEntities: make(map[EntityId]struct{}),
	}
}

func (cs *ClientState) addObservedEntity(entityId EntityId) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.observedEntities[entityId] = struct{}{}
}

func (cs *ClientState) removeObservedEntity(entityId EntityId) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	delete(cs.observedEntities, entityId)
}

func (cs *ClientState) getObservedEntities() []EntityId {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	result := make([]EntityId, 0, len(cs.observedEntities))
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

// ============================================================================
// World
// ============================================================================

type Protocol interface {
	EncodeDelta(delta *ClientDelta) ([]byte, error)
}

type World struct {
	// Entity management
	nextEntityId atomic.Uint64
	entities     map[EntityId]struct{}
	entitiesMu   sync.RWMutex

	// Component storage
	components   map[ComponentType]*componentStore
	componentsMu sync.RWMutex

	// Version tracking
	currentVersion atomic.Uint64

	// Change tracking
	createdEntities   map[EntityId]struct{}
	destroyedEntities map[EntityId]struct{}
	changesMu         sync.RWMutex

	// Delta history
	deltaHistory *DeltaHistory

	// Client management
	clients   map[ClientId]*ClientState
	clientsMu sync.RWMutex
}

func NewWorld() *World {
	return NewWorldWithHistory(300)
}

func NewWorldWithHistory(maxHistoryDeltas int) *World {
	w := &World{
		entities:          make(map[EntityId]struct{}),
		components:        make(map[ComponentType]*componentStore),
		createdEntities:   make(map[EntityId]struct{}),
		destroyedEntities: make(map[EntityId]struct{}),
		deltaHistory:      newDeltaHistory(maxHistoryDeltas),
		clients:           make(map[ClientId]*ClientState),
	}
	w.nextEntityId.Store(1)
	w.currentVersion.Store(0)
	return w
}

// ============================================================================
// Entity Management
// ============================================================================

func (w *World) CreateEntity() EntityId {
	entityId := EntityId(w.nextEntityId.Add(1) - 1)

	w.entitiesMu.Lock()
	w.entities[entityId] = struct{}{}
	w.entitiesMu.Unlock()

	w.changesMu.Lock()
	w.createdEntities[entityId] = struct{}{}
	w.changesMu.Unlock()

	return entityId
}

func (w *World) DestroyEntity(entityId EntityId) error {
	w.entitiesMu.Lock()
	if _, exists := w.entities[entityId]; !exists {
		w.entitiesMu.Unlock()
		return ErrEntityNotFound{entityId}
	}
	delete(w.entities, entityId)
	w.entitiesMu.Unlock()

	w.componentsMu.RLock()
	for _, store := range w.components {
		store.remove(entityId)
	}
	w.componentsMu.RUnlock()

	w.changesMu.Lock()
	w.destroyedEntities[entityId] = struct{}{}
	delete(w.createdEntities, entityId)
	w.changesMu.Unlock()

	return nil
}

func (w *World) EntityExists(entityId EntityId) bool {
	w.entitiesMu.RLock()
	defer w.entitiesMu.RUnlock()
	_, exists := w.entities[entityId]
	return exists
}

// ============================================================================
// Component Management
// ============================================================================

func (w *World) getOrCreateStore(compType ComponentType) *componentStore {
	w.componentsMu.RLock()
	store, exists := w.components[compType]
	w.componentsMu.RUnlock()

	if exists {
		return store
	}

	w.componentsMu.Lock()
	defer w.componentsMu.Unlock()

	// Double-check after acquiring write lock
	if store, exists := w.components[compType]; exists {
		return store
	}

	store = newComponentStore()
	w.components[compType] = store
	return store
}

func (w *World) SetComponent(entityId EntityId, component Component) error {
	if !w.EntityExists(entityId) {
		return ErrEntityNotFound{entityId}
	}

	compType := ComponentType(component.Type())
	store := w.getOrCreateStore(compType)
	store.set(entityId, component)

	return nil
}

func (w *World) GetComponent(entityId EntityId, compType ComponentType) (Component, error) {
	w.componentsMu.RLock()
	store, exists := w.components[compType]
	w.componentsMu.RUnlock()

	if !exists {
		return nil, ErrComponentTypeNotFound{compType}
	}

	component, ok := store.get(entityId)
	if !ok {
		return nil, ErrComponentNotFound{entityId, compType}
	}

	return component, nil
}

func (w *World) RemoveComponent(entityId EntityId, compType ComponentType) error {
	w.componentsMu.RLock()
	store, exists := w.components[compType]
	w.componentsMu.RUnlock()

	if !exists {
		return ErrComponentTypeNotFound{compType}
	}

	if !store.remove(entityId) {
		return ErrComponentNotFound{entityId, compType}
	}

	return nil
}

func (w *World) HasComponent(entityId EntityId, compType ComponentType) bool {
	w.componentsMu.RLock()
	store, exists := w.components[compType]
	w.componentsMu.RUnlock()

	if !exists {
		return false
	}

	_, has := store.get(entityId)
	return has
}

// ============================================================================
// Client Management
// ============================================================================

func (w *World) RegisterClient(clientId ClientId) {
	w.clientsMu.Lock()
	defer w.clientsMu.Unlock()

	if _, exists := w.clients[clientId]; !exists {
		w.clients[clientId] = newClientState()
	}
}

func (w *World) UnregisterClient(clientId ClientId) {
	w.clientsMu.Lock()
	delete(w.clients, clientId)
	w.clientsMu.Unlock()
}

func (w *World) AddObservedEntity(clientId ClientId, entityId EntityId) error {
	w.clientsMu.RLock()
	client, exists := w.clients[clientId]
	w.clientsMu.RUnlock()

	if !exists {
		return ErrClientNotFound{clientId}
	}

	if !w.EntityExists(entityId) {
		return ErrEntityNotFound{entityId}
	}

	client.addObservedEntity(entityId)
	return nil
}

func (w *World) RemoveObservedEntity(clientId ClientId, entityId EntityId) error {
	w.clientsMu.RLock()
	client, exists := w.clients[clientId]
	w.clientsMu.RUnlock()

	if !exists {
		return ErrClientNotFound{clientId}
	}

	client.removeObservedEntity(entityId)
	return nil
}

// ============================================================================
// Snapshot Generation
// ============================================================================

type Snapshot struct {
	Version    uint64
	Entities   []EntityId
	Components map[ComponentType]map[EntityId]Component
}

func (w *World) CreateSnapshot() *Snapshot {
	snapshot := &Snapshot{
		Version:    w.currentVersion.Load(),
		Entities:   make([]EntityId, 0),
		Components: make(map[ComponentType]map[EntityId]Component),
	}

	w.entitiesMu.RLock()
	for entityId := range w.entities {
		snapshot.Entities = append(snapshot.Entities, entityId)
	}
	w.entitiesMu.RUnlock()

	w.componentsMu.RLock()
	for compType, store := range w.components {
		snapshot.Components[compType] = store.getAll()
	}
	w.componentsMu.RUnlock()

	return snapshot
}

func (w *World) CreateSnapshotForClient(clientId ClientId) (*Snapshot, error) {
	w.clientsMu.RLock()
	client, exists := w.clients[clientId]
	w.clientsMu.RUnlock()

	if !exists {
		return nil, ErrClientNotFound{clientId}
	}

	snapshot := &Snapshot{
		Version:    w.currentVersion.Load(),
		Entities:   make([]EntityId, 0),
		Components: make(map[ComponentType]map[EntityId]Component),
	}

	// Only include observed entities
	observedEntities := client.getObservedEntities()
	snapshot.Entities = observedEntities

	w.componentsMu.RLock()
	for compType, store := range w.components {
		snapshot.Components[compType] = make(map[EntityId]Component)
		for _, entityId := range observedEntities {
			if component, ok := store.get(entityId); ok {
				snapshot.Components[compType][entityId] = component
			}
		}
	}
	w.componentsMu.RUnlock()

	client.setLastVersion(snapshot.Version)

	return snapshot, nil
}

// ============================================================================
// Delta Generation
// ============================================================================

type DeltaHistory struct {
	deltas    []HistoricalDelta
	maxDeltas int
	mu        sync.RWMutex
}

type HistoricalDelta struct {
	Version           uint64
	CreatedEntities   map[EntityId]struct{}
	DestroyedEntities map[EntityId]struct{}
	ComponentChanges  map[ComponentType]map[EntityId]Component
}

func newDeltaHistory(maxDeltas int) *DeltaHistory {
	return &DeltaHistory{
		deltas:    make([]HistoricalDelta, 0, maxDeltas),
		maxDeltas: maxDeltas,
	}
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

	oldestVersion := dh.deltas[0].Version

	// Client is too far behind
	if fromVersion < oldestVersion {
		return nil, false
	}

	result := make([]HistoricalDelta, 0)
	for _, delta := range dh.deltas {
		if delta.Version > fromVersion && delta.Version <= toVersion {
			result = append(result, delta)
		}
	}

	return result, true
}

func (w *World) GenerateDeltas() (map[ClientId]*ClientDelta, error) {
	currentVersion := w.currentVersion.Add(1)

	// Store current changes in history
	historicalDelta := w.captureHistoricalDelta(currentVersion)
	w.deltaHistory.add(historicalDelta)

	result := make(map[ClientId]*ClientDelta)

	w.clientsMu.RLock()
	clients := make(map[ClientId]*ClientState, len(w.clients))
	maps.Copy(clients, w.clients)
	w.clientsMu.RUnlock()

	// Generate deltas for each client
	for clientId, clientState := range clients {
		clientDelta := w.generateDeltaForClient(clientState, currentVersion)

		result[clientId] = clientDelta

		if clientDelta.ResyncRequired {
			clientState.setLastVersion(currentVersion)
		}
	}

	// Clear flags & tracking
	w.componentsMu.RLock()
	for _, store := range w.components {
		store.clearDirty()
	}
	w.componentsMu.RUnlock()

	w.changesMu.Lock()
	w.createdEntities = make(map[EntityId]struct{})
	w.destroyedEntities = make(map[EntityId]struct{})
	w.changesMu.Unlock()

	return result, nil
}

func (w *World) captureHistoricalDelta(version uint64) HistoricalDelta {
	delta := HistoricalDelta{
		Version:           version,
		CreatedEntities:   make(map[EntityId]struct{}),
		DestroyedEntities: make(map[EntityId]struct{}),
		ComponentChanges:  make(map[ComponentType]map[EntityId]Component),
	}

	w.changesMu.RLock()
	for entityId := range w.createdEntities {
		delta.CreatedEntities[entityId] = struct{}{}
	}
	for entityId := range w.destroyedEntities {
		delta.DestroyedEntities[entityId] = struct{}{}
	}
	w.changesMu.RUnlock()

	w.componentsMu.RLock()
	for compType, store := range w.components {
		dirtyComponents := store.getDirty()
		if len(dirtyComponents) > 0 {
			delta.ComponentChanges[compType] = dirtyComponents
		}
	}
	w.componentsMu.RUnlock()

	return delta
}

func (w *World) buildDeltaFromHistory(client *ClientState, history []HistoricalDelta, baseVersion, newVersion uint64) *ClientDelta {
	delta := &ClientDelta{
		BaseVersion: baseVersion,
		NewVersion:  newVersion,
		Deltas:      make([]Delta, 0),
	}

	observedSet := make(map[EntityId]struct{})
	for _, entityId := range client.getObservedEntities() {
		observedSet[entityId] = struct{}{}
	}

	addedEntities := make(map[EntityId]struct{})
	destroyedEntities := make(map[EntityId]struct{})
	updatedComponents := make(map[EntityId]map[ComponentType]Component)

	// Merge all historical deltas
	for _, histDelta := range history {
		for entityId := range histDelta.CreatedEntities {
			if _, observed := observedSet[entityId]; observed {
				if _, destroyed := destroyedEntities[entityId]; !destroyed {
					addedEntities[entityId] = struct{}{}
				}
			}
		}

		for entityId := range histDelta.DestroyedEntities {
			if _, observed := observedSet[entityId]; observed {
				destroyedEntities[entityId] = struct{}{}
				delete(addedEntities, entityId)
			}
		}

		for compType, components := range histDelta.ComponentChanges {
			for entityId, component := range components {
				if _, observed := observedSet[entityId]; !observed {
					continue
				}
				if _, destroyed := destroyedEntities[entityId]; destroyed {
					continue
				}

				if updatedComponents[entityId] == nil {
					updatedComponents[entityId] = make(map[ComponentType]Component)
				}
				updatedComponents[entityId][compType] = component
			}
		}
	}

	// Build final delta list
	for entityId := range addedEntities {
		delta.Deltas = append(delta.Deltas, Delta{
			Type:     DeltaEntityCreated,
			EntityId: entityId,
		})
	}

	for entityId := range destroyedEntities {
		delta.Deltas = append(delta.Deltas, Delta{
			Type:     DeltaEntityDestroyed,
			EntityId: entityId,
		})
	}

	for entityId, components := range updatedComponents {
		for _, component := range components {
			delta.Deltas = append(delta.Deltas, Delta{
				Type:      DeltaComponentUpdated,
				EntityId:  entityId,
				Component: component,
			})
		}
	}

	return delta
}

func (w *World) generateDeltaForClient(client *ClientState, version uint64) *ClientDelta {
	clientVersion := client.getLastVersion()

	if clientVersion == version-1 {
		// Client is only one version behind, send current delta
		delta := w.buildDeltaFromCurrent(client, clientVersion, version)
		return delta
	}

	// Client is behind, try to catch up from history
	if clientVersion < version-1 {
		historicalDeltas, found := w.deltaHistory.getRange(clientVersion, version)

		if !found {
			// Client is too far behind, needs full snapshot
			return &ClientDelta{
				ResyncRequired: true,
			}
		}

		delta := w.buildDeltaFromHistory(client, historicalDeltas, clientVersion, version)
		return delta
	}

	// Client version is somehow ahead (shouldn't happen)
	return &ClientDelta{
		ResyncRequired: true,
	}
}

func (w *World) buildDeltaFromCurrent(client *ClientState, baseVersion, newVersion uint64) *ClientDelta {
	delta := &ClientDelta{
		BaseVersion: baseVersion,
		NewVersion:  newVersion,
		Deltas:      make([]Delta, 0),
	}

	observedEntities := client.getObservedEntities()
	observedSet := make(map[EntityId]struct{}, len(observedEntities))
	for _, entityId := range observedEntities {
		observedSet[entityId] = struct{}{}
	}

	w.changesMu.RLock()
	for entityId := range w.createdEntities {
		if _, observed := observedSet[entityId]; observed {
			delta.Deltas = append(delta.Deltas, Delta{
				Type:     DeltaEntityCreated,
				EntityId: entityId,
			})
		}
	}

	for entityId := range w.destroyedEntities {
		if _, observed := observedSet[entityId]; observed {
			delta.Deltas = append(delta.Deltas, Delta{
				Type:     DeltaEntityDestroyed,
				EntityId: entityId,
			})
		}
	}
	w.changesMu.RUnlock()

	w.componentsMu.RLock()
	for _, store := range w.components {
		dirtyComponents := store.getDirty()

		for entityId, component := range dirtyComponents {
			if _, observed := observedSet[entityId]; !observed {
				continue
			}

			deltaType := DeltaComponentUpdated
			// For simplicity, we treat all dirty as updates
			// TODO Add "add" tracking

			delta.Deltas = append(delta.Deltas, Delta{
				Type:      deltaType,
				EntityId:  entityId,
				Component: component,
			})
		}
	}
	w.componentsMu.RUnlock()

	return delta
}

// ============================================================================
// Utility Functions
// ============================================================================

func (w *World) GetCurrentVersion() uint64 {
	return w.currentVersion.Load()
}

func (w *World) GetEntityCount() int {
	w.entitiesMu.RLock()
	defer w.entitiesMu.RUnlock()
	return len(w.entities)
}

func (w *World) GetClientCount() int {
	w.clientsMu.RLock()
	defer w.clientsMu.RUnlock()
	return len(w.clients)
}
