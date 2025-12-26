package mor

import (
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
)

// EntityID ist eine eindeutige Entitäts-ID
type EntityID uint64

// ComponentType identifiziert Komponententypen
type ComponentType uint32

// Component ist das Interface für alle Komponenten
type Component interface {
	Type() ComponentType
}

// System verarbeitet Entities mit bestimmten Komponenten
type System interface {
	// RequiredComponents gibt die benötigten Komponententypen zurück
	RequiredComponents() []ComponentType
	// Update wird pro Frame aufgerufen
	Update(dt float64, entities []EntityID, world *World)
	// Priority bestimmt die Ausführungsreihenfolge (höher = früher)
	Priority() int
}

// Entity repräsentiert eine Spielentität
type Entity struct {
	ID         EntityID
	Components map[ComponentType]Component
	Active     bool
	mu         sync.RWMutex
}

// AddComponent fügt eine Komponente hinzu
func (e *Entity) AddComponent(c Component) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Components[c.Type()] = c
}

// RemoveComponent entfernt eine Komponente
func (e *Entity) RemoveComponent(ct ComponentType) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.Components, ct)
}

// GetComponent gibt eine Komponente zurück
func (e *Entity) GetComponent(ct ComponentType) (Component, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	c, ok := e.Components[ct]
	return c, ok
}

// HasComponents prüft ob alle Komponenten vorhanden sind
func (e *Entity) HasComponents(types ...ComponentType) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, t := range types {
		if _, ok := e.Components[t]; !ok {
			return false
		}
	}
	return true
}

// World ist der Hauptcontainer für das ECS
type World struct {
	*DeltaRecorder

	entities       map[EntityID]*Entity
	systems        []System
	nextEntityID   uint64
	componentIndex map[ComponentType][]EntityID
	mu             sync.RWMutex

	// Event System
	eventHandlers map[string][]EventHandler
	eventQueue    []Event
	eventMu       sync.Mutex

	// Network Sync
	dirtyEntities map[EntityID]bool
	syncMu        sync.RWMutex
}

// NewWorld erstellt eine neue ECS World
func NewWorld() *World {
	deltaRecorder := NewDeltaRecorder()

	return &World{
		DeltaRecorder:  deltaRecorder,
		entities:       make(map[EntityID]*Entity),
		systems:        make([]System, 0),
		componentIndex: make(map[ComponentType][]EntityID),
		eventHandlers:  make(map[string][]EventHandler),
		eventQueue:     make([]Event, 0),
		dirtyEntities:  make(map[EntityID]bool),
	}
}

// CreateEntity erstellt eine neue Entity
func (w *World) CreateEntity() *Entity {
	id := EntityID(atomic.AddUint64(&w.nextEntityID, 1))

	entity := &Entity{
		ID:         id,
		Components: make(map[ComponentType]Component),
		Active:     true,
	}

	w.mu.Lock()
	w.entities[id] = entity
	w.mu.Unlock()

	w.MarkDirty(id)
	w.EmitEvent(Event{Type: "entity_created", EntityID: id})

	return entity
}

// DestroyEntity entfernt eine Entity
func (w *World) DestroyEntity(id EntityID) {
	w.mu.Lock()
	entity, ok := w.entities[id]
	if !ok {
		w.mu.Unlock()
		return
	}

	entity.Active = false
	delete(w.entities, id)

	// Aus Component-Index entfernen
	for componentType := range entity.Components {
		w.removeFromComponentIndex(componentType, id)
	}
	w.mu.Unlock()

	w.MarkDirty(id)
	w.EmitEvent(Event{Type: "entity_destroyed", EntityID: id})
}

// GetEntity gibt eine Entity zurück
func (w *World) GetEntity(id EntityID) (*Entity, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	entity, ok := w.entities[id]
	return entity, ok
}

// AddComponent fügt einer Entity eine Komponente hinzu
func (w *World) AddComponent(entityID EntityID, c Component) error {
	entity, ok := w.GetEntity(entityID)
	if !ok {
		return fmt.Errorf("entity %d not found", entityID)
	}

	entity.AddComponent(c)

	w.mu.Lock()
	w.addToComponentIndex(c.Type(), entityID)
	w.mu.Unlock()

	w.MarkDirty(entityID)
	return nil
}

// RemoveComponent entfernt eine Komponente von einer Entity
func (w *World) RemoveComponent(entityID EntityID, ct ComponentType) error {
	entity, ok := w.GetEntity(entityID)
	if !ok {
		return fmt.Errorf("entity %d not found", entityID)
	}

	entity.RemoveComponent(ct)

	w.mu.Lock()
	w.removeFromComponentIndex(ct, entityID)
	w.mu.Unlock()

	w.MarkDirty(entityID)
	return nil
}

// addToComponentIndex fügt eine Entity zum Component-Index hinzu
func (w *World) addToComponentIndex(ct ComponentType, id EntityID) {
	entities := w.componentIndex[ct]
	if slices.Contains(entities, id) {
		return
	}
	w.componentIndex[ct] = append(entities, id)
}

// removeFromComponentIndex entfernt eine Entity aus dem Component-Index
func (w *World) removeFromComponentIndex(ct ComponentType, id EntityID) {
	entities := w.componentIndex[ct]
	for i, eid := range entities {
		if eid == id {
			w.componentIndex[ct] = append(entities[:i], entities[i+1:]...)
			return
		}
	}
}

// RegisterSystem registriert ein System
func (w *World) RegisterSystem(s System) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Sortiert nach Priorität einfügen
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

// Update führt alle Systeme aus
func (w *World) Update(dt float64) {
	w.mu.RLock()
	systems := make([]System, len(w.systems))
	copy(systems, w.systems)
	w.mu.RUnlock()

	for _, system := range systems {
		entities := w.GetEntitiesWithComponents(system.RequiredComponents()...)
		system.Update(dt, entities, w)
	}

	// Events verarbeiten
	w.ProcessEvents()
}

// GetEntitiesWithComponents gibt alle Entities mit den gegebenen Komponenten zurück
func (w *World) GetEntitiesWithComponents(types ...ComponentType) []EntityID {
	if len(types) == 0 {
		return []EntityID{}
	}

	w.mu.RLock()
	defer w.mu.RUnlock()

	// Starte mit der kleinsten Menge
	var result []EntityID
	minSize := -1
	var minType ComponentType

	for _, t := range types {
		size := len(w.componentIndex[t])
		if minSize == -1 || size < minSize {
			minSize = size
			minType = t
		}
	}

	if minSize == -1 || minSize == 0 {
		return []EntityID{}
	}

	// Prüfe jede Entity aus der kleinsten Menge
	for _, id := range w.componentIndex[minType] {
		entity, ok := w.entities[id]
		if !ok || !entity.Active {
			continue
		}

		if entity.HasComponents(types...) {
			result = append(result, id)
		}
	}

	return result
}

// ============================================================================
// BEISPIEL-KOMPONENTEN UND SYSTEME
// ============================================================================

// Position Component
const (
	ComponentTypePosition ComponentType = 1
	ComponentTypeVelocity ComponentType = 2
	ComponentTypeHealth   ComponentType = 3
	ComponentTypePlayer   ComponentType = 4
)

type PositionComponent struct {
	X, Y, Z float64
}

func (p *PositionComponent) Type() ComponentType {
	return ComponentTypePosition
}

// Velocity Component
type VelocityComponent struct {
	X, Y, Z float64
}

func (v *VelocityComponent) Type() ComponentType {
	return ComponentTypeVelocity
}

// Health Component
type HealthComponent struct {
	Current int
	Max     int
}

func (h *HealthComponent) Type() ComponentType {
	return ComponentTypeHealth
}

// Player Component (Tag)
type PlayerComponent struct {
	PlayerID string
}

func (p *PlayerComponent) Type() ComponentType {
	return ComponentTypePlayer
}

// Movement System
type MovementSystem struct{}

func (m *MovementSystem) RequiredComponents() []ComponentType {
	return []ComponentType{ComponentTypePosition, ComponentTypeVelocity}
}

func (m *MovementSystem) Update(dt float64, entities []EntityID, world *World) {
	for _, id := range entities {
		entity, ok := world.GetEntity(id)
		if !ok {
			continue
		}

		posComp, _ := entity.GetComponent(ComponentTypePosition)
		velComp, _ := entity.GetComponent(ComponentTypeVelocity)

		pos := posComp.(*PositionComponent)
		vel := velComp.(*VelocityComponent)

		pos.X += vel.X * dt
		pos.Y += vel.Y * dt
		pos.Z += vel.Z * dt

		world.MarkDirty(id)
	}
}

func (m *MovementSystem) Priority() int {
	return 100
}

// Health System
type HealthSystem struct{}

func (h *HealthSystem) RequiredComponents() []ComponentType {
	return []ComponentType{ComponentTypeHealth}
}

func (h *HealthSystem) Update(dt float64, entities []EntityID, world *World) {
	for _, id := range entities {
		entity, ok := world.GetEntity(id)
		if !ok {
			continue
		}

		healthComp, _ := entity.GetComponent(ComponentTypeHealth)
		health := healthComp.(*HealthComponent)

		if health.Current <= 0 {
			world.EmitEvent(Event{
				Type:     "entity_died",
				EntityID: id,
				Data:     health,
			})
		}
	}
}

func (h *HealthSystem) Priority() int {
	return 50
}

// Event System
type Event struct {
	Type     string
	EntityID EntityID
	Data     interface{}
}

type EventHandler func(Event)

// RegisterEventHandler registriert einen Event-Handler
func (w *World) RegisterEventHandler(eventType string, handler EventHandler) {
	w.eventMu.Lock()
	defer w.eventMu.Unlock()
	w.eventHandlers[eventType] = append(w.eventHandlers[eventType], handler)
}

// EmitEvent sendet ein Event
func (w *World) EmitEvent(event Event) {
	w.eventMu.Lock()
	w.eventQueue = append(w.eventQueue, event)
	w.eventMu.Unlock()
}

// ProcessEvents verarbeitet alle ausstehenden Events
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

// Network Synchronization mit Delta-System
func (w *World) MarkDirty(id EntityID) {
	w.syncMu.Lock()
	defer w.syncMu.Unlock()
	w.dirtyEntities[id] = true
}

func (w *World) GetDirtyEntities() []EntityID {
	w.syncMu.Lock()
	defer w.syncMu.Unlock()

	result := make([]EntityID, 0, len(w.dirtyEntities))
	for id := range w.dirtyEntities {
		result = append(result, id)
	}
	return result
}

func (w *World) ClearDirty() {
	w.syncMu.Lock()
	defer w.syncMu.Unlock()
	w.dirtyEntities = make(map[EntityID]bool)
}

// Delta System für effiziente Netzwerk-Updates
type DeltaType uint8

const (
	DeltaEntityCreated DeltaType = iota
	DeltaEntityDestroyed
	DeltaComponentAdded
	DeltaComponentUpdated
	DeltaComponentRemoved
)

// Delta repräsentiert eine Änderung im World-State
type Delta struct {
	Type          DeltaType
	EntityID      EntityID
	ComponentType ComponentType
	OldValue      Component
	NewValue      Component
	Timestamp     int64
}

// DeltaRecorder zeichnet alle Änderungen auf
type DeltaRecorder struct {
	deltas    []Delta
	enabled   bool
	mu        sync.RWMutex
	timestamp int64
}

// NewDeltaRecorder erstellt einen neuen Delta-Recorder
func NewDeltaRecorder() *DeltaRecorder {
	return &DeltaRecorder{
		deltas:  make([]Delta, 0),
		enabled: true,
	}
}

// RecordDelta fügt ein Delta hinzu
func (dr *DeltaRecorder) RecordDelta(d Delta) {
	if !dr.enabled {
		return
	}

	dr.mu.Lock()
	defer dr.mu.Unlock()

	d.Timestamp = atomic.AddInt64(&dr.timestamp, 1)
	dr.deltas = append(dr.deltas, d)
}

// GetDeltas gibt alle Deltas zurück
func (dr *DeltaRecorder) GetDeltas() []Delta {
	dr.mu.RLock()
	defer dr.mu.RUnlock()

	result := make([]Delta, len(dr.deltas))
	copy(result, dr.deltas)
	return result
}

// GetDeltasSince gibt Deltas seit einem Timestamp zurück
func (dr *DeltaRecorder) GetDeltasSince(timestamp int64) []Delta {
	dr.mu.RLock()
	defer dr.mu.RUnlock()

	result := make([]Delta, 0)
	for _, d := range dr.deltas {
		if d.Timestamp > timestamp {
			result = append(result, d)
		}
	}
	return result
}

// ClearDeltas löscht alle Deltas
func (dr *DeltaRecorder) ClearDeltas() {
	dr.mu.Lock()
	defer dr.mu.Unlock()
	dr.deltas = dr.deltas[:0]
}

// ClearDeltasBefore löscht Deltas vor einem Timestamp
func (dr *DeltaRecorder) ClearDeltasBefore(timestamp int64) {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	newDeltas := make([]Delta, 0)
	for _, d := range dr.deltas {
		if d.Timestamp >= timestamp {
			newDeltas = append(newDeltas, d)
		}
	}
	dr.deltas = newDeltas
}

// Enable aktiviert die Delta-Aufzeichnung
func (dr *DeltaRecorder) Enable() {
	dr.mu.Lock()
	defer dr.mu.Unlock()
	dr.enabled = true
}

// Disable deaktiviert die Delta-Aufzeichnung
func (dr *DeltaRecorder) Disable() {
	dr.mu.Lock()
	defer dr.mu.Unlock()
	dr.enabled = false
}

// GetCurrentTimestamp gibt den aktuellen Timestamp zurück
func (dr *DeltaRecorder) GetCurrentTimestamp() int64 {
	return atomic.LoadInt64(&dr.timestamp)
}

// Query Builder für komplexe Abfragen
type Query struct {
	world    *World
	required []ComponentType
	excluded []ComponentType
}

func (w *World) Query() *Query {
	return &Query{world: w}
}

func (q *Query) With(types ...ComponentType) *Query {
	q.required = append(q.required, types...)
	return q
}

func (q *Query) Without(types ...ComponentType) *Query {
	q.excluded = append(q.excluded, types...)
	return q
}

func (q *Query) Execute() []EntityID {
	candidates := q.world.GetEntitiesWithComponents(q.required...)

	if len(q.excluded) == 0 {
		return candidates
	}

	result := make([]EntityID, 0, len(candidates))
	for _, id := range candidates {
		entity, ok := q.world.GetEntity(id)
		if !ok {
			continue
		}

		hasExcluded := false
		for _, et := range q.excluded {
			if _, has := entity.GetComponent(et); has {
				hasExcluded = true
				break
			}
		}

		if !hasExcluded {
			result = append(result, id)
		}
	}

	return result
}
