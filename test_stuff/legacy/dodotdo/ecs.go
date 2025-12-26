package ecs

import (
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------
// Basis-ECS (ursprünglicher Code)
// ---------------------------

type EntityId uint64

type ComponentType string

type Component interface {
	Type() ComponentType
	Serialize() ([]byte, error)
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

	// ---------------------------
	// Sync/Server-Authoritative Additionen
	// ---------------------------

	// Per-entity/per-component Versions um Deltas effizient zu bauen.
	// componentVersions[entityId][componentType] = versionCounter
	componentVersions map[EntityId]map[ComponentType]uint64
	componentVerMu    sync.RWMutex

	// Client-Contexten
	clients   map[ClientId]*ClientSyncContext
	clientsMu sync.RWMutex

	// Global tick (server simulation tick counter)
	tick uint64
}

func NewWorld() *World {
	return &World{
		entities:          make(map[EntityId]*Entity),
		compIndex:         make(map[ComponentType][]EntityId),
		dirtyEntities:     make(map[EntityId]bool),
		eventHandlers:     make(map[string][]EventHandler),
		componentVersions: make(map[EntityId]map[ComponentType]uint64),
		clients:           make(map[ClientId]*ClientSyncContext),
	}
}

func (w *World) CreateEntity() (EntityId, *Entity) {
	id := EntityId(atomic.AddUint64(&w.nextEntityId, 1))

	entity := &Entity{
		components: make(map[ComponentType]Component),
	}

	w.mu.Lock()
	w.entities[id] = entity
	// init version map
	w.componentVerMu.Lock()
	w.componentVersions[id] = make(map[ComponentType]uint64)
	w.componentVerMu.Unlock()
	w.mu.Unlock()

	w.MarkDirty(id)
	w.EmitEvent(Event{Type: "entity_created", EntityId: id})

	return id, entity
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

	// remove version map
	w.componentVerMu.Lock()
	delete(w.componentVersions, id)
	w.componentVerMu.Unlock()

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

	// bump component version
	w.bumpComponentVersion(eid, c.Type())

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

	// bump version to indicate removal
	w.bumpComponentVersion(eid, ct)

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
	// increment tick
	atomic.AddUint64(&w.tick, 1)

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

// ---------------------------
// Component-Versionierung (für Delta-Bildung)
// ---------------------------

// bumpComponentVersion erhöht die Versionsnummer für (entity, component).
// Wird aufgerufen bei Add/Remove/Änderung der Komponente.
func (w *World) bumpComponentVersion(eid EntityId, ct ComponentType) {
	w.componentVerMu.Lock()
	defer w.componentVerMu.Unlock()
	m, ok := w.componentVersions[eid]
	if !ok {
		m = make(map[ComponentType]uint64)
		w.componentVersions[eid] = m
	}
	m[ct] = m[ct] + 1
}

// getComponentVersion returns current version for (entity, component)
func (w *World) getComponentVersion(eid EntityId, ct ComponentType) uint64 {
	w.componentVerMu.RLock()
	defer w.componentVerMu.RUnlock()
	if m, ok := w.componentVersions[eid]; ok {
		return m[ct]
	}
	return 0
}

// setComponentVersion sets explicit version (used by snapshot restore or admin ops)
func (w *World) setComponentVersion(eid EntityId, ct ComponentType, v uint64) {
	w.componentVerMu.Lock()
	defer w.componentVerMu.Unlock()
	m, ok := w.componentVersions[eid]
	if !ok {
		m = make(map[ComponentType]uint64)
		w.componentVersions[eid] = m
	}
	m[ct] = v
}

// ---------------------------
// Client-Sync-Strukturen
// ---------------------------

type ClientId string

// InputMessage: was der Client sendet (vereinfachtes Modell)
type InputMessage struct {
	Seq       uint32    // Sequenznummer des Inputs
	Timestamp time.Time // Clientzeitstempel
	Actions   any       // Struktur mit Aktionen (Bewegung, Schießen, ...)
	RecvAt    time.Time // Server-Empfangszeit (gefüllt serverseitig)
}

// PendingDelta beschreibt ein gesendetes Delta, das noch nicht bestätigt wurde.
type PendingDelta struct {
	Tick     uint64
	Checksum uint32 // optional: checksum des resultierenden States nach Anwendung
	SentAt   time.Time
	// payload kann nen Verweis/ID zur serialisierten Nachricht enthalten
}

// ClientSyncContext enthält alle serverseitig benötigten Daten pro Client.
type ClientSyncContext struct {
	// Snapshot / replication context
	lastConfirmedTick uint64                                // bis zu welchem Tick der Client bestätigt hat
	lastSentTick      uint64                                // letzter gesendeter Tick
	baseSnapshot      map[EntityId]map[ComponentType]uint64 // welche versionen der Client kennt (component versions)
	baseMu            sync.RWMutex

	// pending deltas und retransmission
	pendingDeltas []PendingDelta
	pendingMu     sync.Mutex

	// interest / visibility
	visibleEntities map[EntityId]bool
	visibleMu       sync.RWMutex

	// input buffering for reconciliation
	inputBuffer []InputMessage
	inputMu     sync.Mutex
	// last confirmed input seq from server processing
	lastConfirmedInputSeq uint32
}

func NewClientSyncContext(id ClientId, token, ip string) *ClientSyncContext {
	return &ClientSyncContext{
		baseSnapshot:    make(map[EntityId]map[ComponentType]uint64),
		visibleEntities: make(map[EntityId]bool),
		inputBuffer:     make([]InputMessage, 0),
	}
}

// ---------------------------
// Delta-Paket Datenmodell (konzeptionell)
// ---------------------------

type DeltaPacket struct {
	Tick          uint64
	BaseTick      uint64
	Checksum      uint32
	Entities      []EntityDelta
	Compressed    bool
	CompressionId string
	CreatedAt     time.Time
}

type EntityDelta struct {
	EntityId     EntityId
	EntityFlags  EntityFlags
	Components   []ComponentDelta
	LastVersions map[ComponentType]uint64 // versionen wie beim Client (für Debug/validation)
}

type ComponentDelta struct {
	ComponentType ComponentType
	FieldMask     uint64 // bitmask für geänderte Felder (falls bekannt)
	// For simplicity: full component payload (in a real system we'd do field-level deltas + quantization)
	Payload any
	Version uint64
}

type EntityFlags uint8

const (
	EntityFlagNone      EntityFlags = 0
	EntityFlagCreated   EntityFlags = 1 << 0
	EntityFlagDestroyed EntityFlags = 1 << 1
	EntityFlagUpdated   EntityFlags = 1 << 2
)

// ---------------------------
// World <-> Client management
// ---------------------------

func (w *World) RegisterClient(id ClientId, token, ip string) *ClientSyncContext {
	ctx := NewClientSyncContext(id, token, ip)
	w.clientsMu.Lock()
	w.clients[id] = ctx
	w.clientsMu.Unlock()
	return ctx
}

func (w *World) UnregisterClient(id ClientId) {
	w.clientsMu.Lock()
	delete(w.clients, id)
	w.clientsMu.Unlock()
}

// AppendInput wird serverseitig aufgerufen, wenn vom Client Eingaben ankommen.
func (w *World) AppendInput(clientId ClientId, input InputMessage) {
	w.clientsMu.RLock()
	ctx, ok := w.clients[clientId]
	w.clientsMu.RUnlock()
	if !ok {
		return
	}
	input.RecvAt = time.Now()
	ctx.inputMu.Lock()
	ctx.inputBuffer = append(ctx.inputBuffer, input)
	ctx.inputMu.Unlock()
}

// ConfirmInputSeq markiert, bis welche Input-Seq der Server für diesen Client verarbeitet hat.
func (w *World) ConfirmInputSeq(clientId ClientId, seq uint32) {
	w.clientsMu.RLock()
	ctx, ok := w.clients[clientId]
	w.clientsMu.RUnlock()
	if !ok {
		return
	}
	ctx.inputMu.Lock()
	if seq > ctx.lastConfirmedInputSeq {
		ctx.lastConfirmedInputSeq = seq
	}
	// optional: trim input buffer
	newBuf := ctx.inputBuffer[:0]
	for _, in := range ctx.inputBuffer {
		if in.Seq > seq {
			newBuf = append(newBuf, in)
		}
	}
	ctx.inputBuffer = newBuf
	ctx.inputMu.Unlock()
}

// AcknowledgeTick vermerkt, dass Client einen Tick-Delta/Snapshot erhalten und angewendet hat.
func (w *World) AcknowledgeTick(clientId ClientId, tick uint64, checksum uint32) {
	w.clientsMu.RLock()
	ctx, ok := w.clients[clientId]
	w.clientsMu.RUnlock()
	if !ok {
		return
	}
	ctx.baseMu.Lock()
	// Wenn Client bestätigt, setzen wir lastConfirmedTick und aktualisieren baseSnapshot (vereinfachter Ansatz).
	if tick > ctx.lastConfirmedTick {
		ctx.lastConfirmedTick = tick
	}
	// Entferne bestätigte pending deltas
	ctx.pendingMu.Lock()
	newPending := ctx.pendingDeltas[:0]
	for _, p := range ctx.pendingDeltas {
		if p.Tick <= tick {
			// confirmed -> drop
			continue
		}
		newPending = append(newPending, p)
	}
	ctx.pendingDeltas = newPending
	ctx.pendingMu.Unlock()
	ctx.baseMu.Unlock()
}

// RecordSentDelta speichert metadata zu einem versendeten Delta
func (w *World) RecordSentDelta(clientId ClientId, pd PendingDelta) {
	w.clientsMu.RLock()
	ctx, ok := w.clients[clientId]
	w.clientsMu.RUnlock()
	if !ok {
		return
	}
	ctx.pendingMu.Lock()
	ctx.pendingDeltas = append(ctx.pendingDeltas, pd)
	ctx.pendingMu.Unlock()
	ctx.lastSentTick = pd.Tick
}

// SetVisibleEntities setzt die Sichtbarkeitsmenge (Interest Management)
func (w *World) SetVisibleEntities(clientId ClientId, visible []EntityId) {
	w.clientsMu.RLock()
	ctx, ok := w.clients[clientId]
	w.clientsMu.RUnlock()
	if !ok {
		return
	}
	ctx.visibleMu.Lock()
	ctx.visibleEntities = make(map[EntityId]bool, len(visible))
	for _, eid := range visible {
		ctx.visibleEntities[eid] = true
	}
	ctx.visibleMu.Unlock()
}

// IsEntityVisible prüft, ob eine Entity für einen Client relevant ist.
func (w *World) IsEntityVisible(clientId ClientId, eid EntityId) bool {
	w.clientsMu.RLock()
	ctx, ok := w.clients[clientId]
	w.clientsMu.RUnlock()
	if !ok {
		return false
	}
	ctx.visibleMu.RLock()
	defer ctx.visibleMu.RUnlock()
	return ctx.visibleEntities[eid]
}

// ---------------------------
// Delta-Bildung (konzeptionell)
// ---------------------------

// BuildDeltaForClient erzeugt ein DeltaPacket für den Client basierend auf dessen baseSnapshot.
// Diese Funktion ist bewusst synchron und in-memory — Serialisierung/Compression muss extern erfolgen.
func (w *World) BuildDeltaForClient(clientId ClientId) *DeltaPacket {
	w.clientsMu.RLock()
	ctx, ok := w.clients[clientId]
	w.clientsMu.RUnlock()
	if !ok {
		return nil
	}

	// snapshot base tick
	ctx.baseMu.RLock()
	baseTick := ctx.lastConfirmedTick
	ctx.baseMu.RUnlock()

	packet := &DeltaPacket{
		Tick:      atomic.LoadUint64(&w.tick),
		BaseTick:  baseTick,
		Entities:  make([]EntityDelta, 0),
		CreatedAt: time.Now(),
	}

	// For each dirty entity: compare versions against client's base snapshot
	dirty := w.GetDirtyEntities()
	for _, eid := range dirty {
		// interest culling: skip entities the client doesn't need
		if !w.IsEntityVisible(clientId, eid) {
			continue
		}

		// collect component deltas
		w.componentVerMu.RLock()
		compMap, ok := w.componentVersions[eid]
		w.componentVerMu.RUnlock()
		if !ok {
			// entity might be deleted or has no components; still include destroyed/created flags depending on existence
			// check existence
			w.mu.RLock()
			_, exists := w.entities[eid]
			w.mu.RUnlock()
			var flags EntityFlags
			if !exists {
				flags = EntityFlagDestroyed
			} else {
				flags = EntityFlagUpdated
			}
			packet.Entities = append(packet.Entities, EntityDelta{
				EntityId:    eid,
				EntityFlags: flags,
				Components:  nil,
			})
			continue
		}

		// compare per component
		ed := EntityDelta{
			EntityId:     eid,
			EntityFlags:  EntityFlagNone,
			Components:   make([]ComponentDelta, 0),
			LastVersions: make(map[ComponentType]uint64),
		}

		// ensure base snapshot map exists
		ctx.baseMu.RLock()
		baseForEntity, baseHas := ctx.baseSnapshot[eid]
		ctx.baseMu.RUnlock()

		// iterate all components present on server
		for ctype, ver := range compMap {
			ed.LastVersions[ctype] = ver
			baseVer := uint64(0)
			if baseHas {
				baseVer = baseForEntity[ctype]
			}
			if ver != baseVer {
				// component changed (or added/removed)
				// fetch component payload
				w.mu.RLock()
				entity, ok := w.entities[eid]
				w.mu.RUnlock()
				var payload any
				if ok {
					// if component exists, include full component payload (simplified)
					if comp, found := entity.GetComponent(ctype); found {
						payload = comp
					} else {
						// component removal -> payload nil means removed
						payload = nil
					}
				} else {
					// entity deleted -> component removed
					payload = nil
				}

				cd := ComponentDelta{
					ComponentType: ctype,
					FieldMask:     0, // unknown here; in real impl. we compute per-field masks
					Payload:       payload,
					Version:       ver,
				}
				ed.Components = append(ed.Components, cd)
			}
		}

		// Also check components that the client knew but server no longer has (component removed)
		if baseHas {
			for bctype, bver := range baseForEntity {
				if _, ok := compMap[bctype]; !ok {
					// component was removed on server
					cd := ComponentDelta{
						ComponentType: bctype,
						FieldMask:     0,
						Payload:       nil, // nil payload indicates removal
						Version:       0,
					}
					ed.Components = append(ed.Components, cd)
					_ = bver // could be used for logging
				}
			}
		}

		// set flags based on components
		if len(ed.Components) > 0 {
			ed.EntityFlags |= EntityFlagUpdated
		}
		// check created
		w.mu.RLock()
		_, exists := w.entities[eid]
		w.mu.RUnlock()
		if exists {
			// if client had no snapshot for this entity, mark created
			if !baseHas {
				ed.EntityFlags |= EntityFlagCreated
			}
		} else {
			ed.EntityFlags |= EntityFlagDestroyed
		}

		// only include entities that have some change or special flags
		if ed.EntityFlags != EntityFlagNone || len(ed.Components) > 0 {
			packet.Entities = append(packet.Entities, ed)
		}
	}

	// Optional: compute checksum of resulting state after delta application - omitted here (fill externally)
	return packet
}

// ApplyDeltaToClientBaseSnapshot aktualisiert die serverseitige baseSnapshot für einen Client nach Versand/Bestätigung.
// Diese Funktion wird z.B. vom Netzwerk-Layer nach dem Bestätigen des Deltas aufgerufen.
func (w *World) ApplyDeltaToClientBaseSnapshot(clientId ClientId, delta *DeltaPacket) {
	w.clientsMu.RLock()
	ctx, ok := w.clients[clientId]
	w.clientsMu.RUnlock()
	if !ok || delta == nil {
		return
	}

	ctx.baseMu.Lock()
	defer ctx.baseMu.Unlock()

	// ensure maps exist for each entity
	for _, ed := range delta.Entities {
		if ed.EntityFlags&EntityFlagDestroyed != 0 {
			// remove entity from base snapshot
			delete(ctx.baseSnapshot, ed.EntityId)
			continue
		}
		m, ok := ctx.baseSnapshot[ed.EntityId]
		if !ok {
			m = make(map[ComponentType]uint64)
			ctx.baseSnapshot[ed.EntityId] = m
		}
		for _, cd := range ed.Components {
			if cd.Payload == nil {
				// removed component
				delete(m, cd.ComponentType)
			} else {
				m[cd.ComponentType] = cd.Version
			}
		}
	}
	// update lastConfirmedTick to delta.Tick
	if delta.Tick > ctx.lastConfirmedTick {
		ctx.lastConfirmedTick = delta.Tick
	}
}

// ---------------------------
// Hilfs-APIs / Utilities
// ---------------------------

// GetClientSyncContext returns the context or nil
func (w *World) GetClientSyncContext(clientId ClientId) *ClientSyncContext {
	w.clientsMu.RLock()
	defer w.clientsMu.RUnlock()
	return w.clients[clientId]
}

// UpdateEntityComponent should be called whenever a component's internal state is modified in-place
// (z.B. Velocity.X = ...). Das ist wichtig: wenn Komponenten intern mutiert werden ohne AddComponent,
// muss diese Funktion aufgerufen werden, damit die Version erhöht und Deltas erzeugt werden.
func (w *World) UpdateEntityComponent(eid EntityId, ct ComponentType) {
	// increase version and mark dirty
	w.bumpComponentVersion(eid, ct)
	w.MarkDirty(eid)
}

// BuildAndRecordAndMarkSent: helper, baut Delta, recordet pending delta (metadata) und returned das Delta.
// Networking layer kann das Delta serialisieren und senden.
func (w *World) BuildAndRecordForClient(clientId ClientId) *DeltaPacket {
	packet := w.BuildDeltaForClient(clientId)
	if packet == nil {
		return nil
	}
	// record pending
	pd := PendingDelta{
		Tick:     packet.Tick,
		Checksum: packet.Checksum,
		SentAt:   time.Now(),
	}
	w.RecordSentDelta(clientId, pd)
	return packet
}

type Vector2 struct {
	X, Y float64
}

func (v Vector2) Serialize() ([]byte, error) {
	return []byte{}, nil
}

type Position struct{ Vector2 }

func (p Position) Type() ComponentType {
	return "position"
}

func ExampleA() {
	w := NewWorld()

	e, _ := w.CreateEntity()
	w.AddComponent(e, Position{Vector2{0, 0}})
}
