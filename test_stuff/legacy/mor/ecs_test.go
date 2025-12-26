package mor

import (
	"testing"
	"time"
)

// ============================================================================
// UNIT TESTS
// ============================================================================

func TestWorldCreation(t *testing.T) {
	world := NewWorld()
	if world == nil {
		t.Fatal("World should not be nil")
	}
}

func TestEntityCreation(t *testing.T) {
	world := NewWorld()
	entity := world.CreateEntity()

	if entity.ID == 0 {
		t.Error("Entity ID should not be 0")
	}

	if !entity.Active {
		t.Error("Entity should be active")
	}

	retrieved, ok := world.GetEntity(entity.ID)
	if !ok {
		t.Error("Entity should be retrievable")
	}

	if retrieved.ID != entity.ID {
		t.Error("Retrieved entity ID should match")
	}
}

func TestComponentAddRemove(t *testing.T) {
	world := NewWorld()
	entity := world.CreateEntity()

	pos := &PositionComponent{X: 10, Y: 20, Z: 30}
	world.AddComponent(entity.ID, pos)

	retrieved, ok := entity.GetComponent(ComponentTypePosition)
	if !ok {
		t.Error("Component should be retrievable")
	}

	posRetrieved := retrieved.(*PositionComponent)
	if posRetrieved.X != 10 || posRetrieved.Y != 20 {
		t.Error("Component values should match")
	}

	world.RemoveComponent(entity.ID, ComponentTypePosition)
	_, ok = entity.GetComponent(ComponentTypePosition)
	if ok {
		t.Error("Component should be removed")
	}
}

func TestSystemExecution(t *testing.T) {
	world := NewWorld()
	world.RegisterSystem(&MovementSystem{})

	entity := world.CreateEntity()
	world.AddComponent(entity.ID, &PositionComponent{X: 0, Y: 0, Z: 0})
	world.AddComponent(entity.ID, &VelocityComponent{X: 10, Y: 5, Z: 0})

	world.Update(1.0)

	retrieved, _ := entity.GetComponent(ComponentTypePosition)
	pos := retrieved.(*PositionComponent)

	if pos.X != 10 || pos.Y != 5 {
		t.Errorf("Position should be updated, got X=%f, Y=%f", pos.X, pos.Y)
	}
}

func TestQueryBuilder(t *testing.T) {
	world := NewWorld()

	e1 := world.CreateEntity()
	world.AddComponent(e1.ID, &PositionComponent{})
	world.AddComponent(e1.ID, &VelocityComponent{})

	e2 := world.CreateEntity()
	world.AddComponent(e2.ID, &PositionComponent{})

	e3 := world.CreateEntity()
	world.AddComponent(e3.ID, &PositionComponent{})
	world.AddComponent(e3.ID, &VelocityComponent{})
	world.AddComponent(e3.ID, &HealthComponent{Current: 100, Max: 100})

	// Query: Entities with Position and Velocity but without Health
	results := world.Query().
		With(ComponentTypePosition, ComponentTypeVelocity).
		Without(ComponentTypeHealth).
		Execute()

	if len(results) != 1 || results[0] != e1.ID {
		t.Errorf("Query should return only e1, got %d entities", len(results))
	}
}

func TestEventSystem(t *testing.T) {
	world := NewWorld()

	eventReceived := false
	var receivedEntityID EntityID

	world.RegisterEventHandler("test_event", func(e Event) {
		eventReceived = true
		receivedEntityID = e.EntityID
	})

	entity := world.CreateEntity()
	world.EmitEvent(Event{Type: "test_event", EntityID: entity.ID})
	world.ProcessEvents()

	if !eventReceived {
		t.Error("Event should be received")
	}

	if receivedEntityID != entity.ID {
		t.Error("Received entity ID should match")
	}
}

func TestDirtyTracking(t *testing.T) {
	world := NewWorld()
	entity := world.CreateEntity()

	dirty := world.GetDirtyEntities()
	if len(dirty) == 0 {
		t.Error("New entity should be dirty")
	}

	world.ClearDirty()
	dirty = world.GetDirtyEntities()
	if len(dirty) != 0 {
		t.Error("Dirty list should be clear")
	}

	world.AddComponent(entity.ID, &PositionComponent{})
	dirty = world.GetDirtyEntities()
	if len(dirty) == 0 {
		t.Error("Entity should be dirty after component addition")
	}
}

func TestConcurrentAccess(t *testing.T) {
	world := NewWorld()

	// Erstelle viele Entities parallel
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				entity := world.CreateEntity()
				world.AddComponent(entity.ID, &PositionComponent{X: float64(j)})
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Sollte 1000 Entities haben
	entities := world.GetEntitiesWithComponents(ComponentTypePosition)
	if len(entities) != 1000 {
		t.Errorf("Expected 1000 entities, got %d", len(entities))
	}
}

func BenchmarkEntityCreation(b *testing.B) {
	world := NewWorld()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		world.CreateEntity()
	}
}

func BenchmarkComponentAccess(b *testing.B) {
	world := NewWorld()
	entity := world.CreateEntity()
	world.AddComponent(entity.ID, &PositionComponent{X: 10, Y: 20})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entity.GetComponent(ComponentTypePosition)
	}
}

func BenchmarkSystemUpdate(b *testing.B) {
	world := NewWorld()
	world.RegisterSystem(&MovementSystem{})

	for i := 0; i < 1000; i++ {
		entity := world.CreateEntity()
		world.AddComponent(entity.ID, &PositionComponent{})
		world.AddComponent(entity.ID, &VelocityComponent{X: 1, Y: 1})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		world.Update(0.016)
	}
}

// ============================================================================
// DELTA SYSTEM TESTS
// ============================================================================

func TestDeltaRecording(t *testing.T) {
	world := NewWorld()

	// Delta-Tracking ist standardmäßig aktiviert
	world.CreateEntity()

	deltas := world.GetDeltas()
	if len(deltas) != 1 {
		t.Errorf("Expected 1 delta (entity created), got %d", len(deltas))
	}

	if deltas[0].Type != DeltaEntityCreated {
		t.Error("First delta should be entity creation")
	}
}

func TestDeltaComponentChanges(t *testing.T) {
	world := NewWorld()
	entity := world.CreateEntity()

	world.ClearDeltas() // Clear creation delta

	// Component hinzufügen
	pos := &PositionComponent{X: 10, Y: 20, Z: 30}
	world.AddComponent(entity.ID, pos)

	deltas := world.GetDeltas()
	if len(deltas) != 1 {
		t.Errorf("Expected 1 delta (component added), got %d", len(deltas))
	}

	if deltas[0].Type != DeltaComponentAdded {
		t.Error("Delta should be component added")
	}

	// Component aktualisieren
	world.AddComponent(entity.ID, &PositionComponent{X: 15, Y: 25, Z: 35})

	deltas = world.GetDeltas()
	if len(deltas) != 2 {
		t.Errorf("Expected 2 deltas, got %d", len(deltas))
	}

	if deltas[1].Type != DeltaComponentUpdated {
		t.Error("Second delta should be component updated")
	}

	// Component entfernen
	world.RemoveComponent(entity.ID, ComponentTypePosition)

	deltas = world.GetDeltas()
	if len(deltas) != 3 {
		t.Errorf("Expected 3 deltas, got %d", len(deltas))
	}

	if deltas[2].Type != DeltaComponentRemoved {
		t.Error("Third delta should be component removed")
	}
}

func TestDeltaTimestamps(t *testing.T) {
	world := NewWorld()

	timestamp1 := world.GetCurrentTimestamp()

	entity := world.CreateEntity()

	timestamp2 := world.GetCurrentTimestamp()

	if timestamp2 <= timestamp1 {
		t.Error("Timestamp should increase after creating entity")
	}

	deltas := world.GetDeltasSince(timestamp1)
	if len(deltas) != 1 {
		t.Errorf("Expected 1 delta since timestamp, got %d", len(deltas))
	}

	world.AddComponent(entity.ID, &PositionComponent{})

	deltas = world.GetDeltasSince(timestamp2)
	if len(deltas) != 1 {
		t.Errorf("Expected 1 delta since second timestamp, got %d", len(deltas))
	}
}

/*func TestDeltaCompression(t *testing.T) {
	world := NewWorld()
	entity := world.CreateEntity()

	// Mehrere Updates auf die gleiche Komponente
	world.AddComponent(entity.ID, &PositionComponent{X: 1, Y: 1, Z: 1})
	world.AddComponent(entity.ID, &PositionComponent{X: 2, Y: 2, Z: 2})
	world.AddComponent(entity.ID, &PositionComponent{X: 3, Y: 3, Z: 3})

	deltas := world.GetDeltas()
	if len(deltas) != 4 { // 1 creation + 3 updates
		t.Errorf("Expected 4 deltas, got %d", len(deltas))
	}

	// Komprimieren
	compressor := &DeltaCompressor{}
	compressed := compressor.CompressDeltas(deltas)

	// Sollte auf 2 Deltas reduziert werden (creation + letztes update)
	if len(compressed) != 2 {
		t.Errorf("Expected 2 compressed deltas, got %d", len(compressed))
	}
}

func TestDeltaCompressionWithDeletion(t *testing.T) {
	world := NewWorld()
	entity := world.CreateEntity()

	world.AddComponent(entity.ID, &PositionComponent{X: 1, Y: 1, Z: 1})
	world.AddComponent(entity.ID, &VelocityComponent{X: 5, Y: 5, Z: 0})

	// Entity wieder löschen
	world.DestroyEntity(entity.ID)

	deltas := world.GetDeltas()

	// Komprimieren
	compressor := &DeltaCompressor{}
	compressed := compressor.CompressDeltas(deltas)

	// Sollte alles außer Löschung entfernen (oder komplett leer wenn creation+deletion sich aufheben)
	if len(compressed) > 1 {
		t.Errorf("Expected at most 1 compressed delta, got %d", len(compressed))
	}
}*/

/*func TestDeltaBatch(t *testing.T) {
	world := NewWorld()

	startTimestamp := world.GetCurrentDeltaTimestamp()

	// Mehrere Änderungen
	e1 := world.CreateEntity()
	world.AddComponent(e1.ID, &PositionComponent{})

	e2 := world.CreateEntity()
	world.AddComponent(e2.ID, &HealthComponent{Current: 100, Max: 100})

	// Batch erstellen
	batch := world.CreateDeltaBatch(startTimestamp)

	if batch.StartTimestamp != startTimestamp {
		t.Error("Batch start timestamp should match")
	}

	if len(batch.Deltas) != 4 { // 2 creations + 2 component adds
		t.Errorf("Expected 4 deltas in batch, got %d", len(batch.Deltas))
	}

	if batch.EndTimestamp <= batch.StartTimestamp {
		t.Error("End timestamp should be greater than start timestamp")
	}
}*/

/*func TestDeltaApply(t *testing.T) {
	// Quelle World
	source := NewWorld()
	entity := source.CreateEntity()
	source.AddComponent(entity.ID, &PositionComponent{X: 10, Y: 20, Z: 30})

	deltas := source.GetDeltas()

	// Ziel World
	target := NewWorld()
	target.DisableDeltaTracking() // Keine Deltas beim Anwenden erzeugen

	// Deltas anwenden
	err := target.ApplyDeltas(deltas)
	if err != nil {
		t.Errorf("Failed to apply deltas: %v", err)
	}

	// Prüfen ob Entity und Component existieren
	targetEntity, ok := target.GetEntity(entity.ID)
	if !ok {
		t.Error("Entity should exist in target world")
	}

	posComp, ok := targetEntity.GetComponent(ComponentTypePosition)
	if !ok {
		t.Error("Position component should exist")
	}

	pos := posComp.(*PositionComponent)
	if pos.X != 10 || pos.Y != 20 || pos.Z != 30 {
		t.Error("Component values should match")
	}
}*/

func TestDeltaClearBefore(t *testing.T) {
	world := NewWorld()

	// Mehrere Entities erstellen
	for range 5 {
		world.CreateEntity()
		time.Sleep(1 * time.Millisecond)
	}

	midTimestamp := world.GetCurrentTimestamp()

	for i := 0; i < 5; i++ {
		world.CreateEntity()
		time.Sleep(1 * time.Millisecond)
	}

	deltas := world.GetDeltas()
	if len(deltas) != 10 {
		t.Errorf("Expected 10 deltas, got %d", len(deltas))
	}

	// Alte Deltas löschen
	world.ClearDeltasBefore(midTimestamp)

	deltas = world.GetDeltas()
	if len(deltas) >= 10 {
		t.Error("Old deltas should be cleared")
	}
}

func BenchmarkDeltaRecording(b *testing.B) {
	world := NewWorld()
	entity := world.CreateEntity()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		world.AddComponent(entity.ID, &PositionComponent{X: float64(i)})
	}
}

/*func BenchmarkDeltaCompression(b *testing.B) {
	world := NewWorld()

	// Viele Deltas erzeugen
	for i := 0; i < 1000; i++ {
		entity := world.CreateEntity()
		world.AddComponent(entity.ID, &PositionComponent{X: float64(i)})
	}

	deltas := world.GetDeltas()
	compressor := &DeltaCompressor{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		compressor.CompressDeltas(deltas)
	}
}*/

// ============================================================================
// VOLLSTÄNDIGES BEISPIEL: MULTIPLAYER GAME
// ============================================================================

// Beispiel für ein einfaches Multiplayer-Spiel
func ExampleMultiplayerGame() {
	// World erstellen
	world := NewWorld()

	// Systeme registrieren
	world.RegisterSystem(&MovementSystem{})
	world.RegisterSystem(&HealthSystem{})

	// Event Handler für gestorbene Entities
	world.RegisterEventHandler("entity_died", func(e Event) {
		println("Entity died:", e.EntityID)
		world.DestroyEntity(e.EntityID)
	})

	// Spieler erstellen
	player1 := world.CreateEntity()
	world.AddComponent(player1.ID, &PositionComponent{X: 0, Y: 0, Z: 0})
	world.AddComponent(player1.ID, &VelocityComponent{X: 5, Y: 0, Z: 0})
	world.AddComponent(player1.ID, &HealthComponent{Current: 100, Max: 100})
	world.AddComponent(player1.ID, &PlayerComponent{PlayerID: "player_1"})

	player2 := world.CreateEntity()
	world.AddComponent(player2.ID, &PositionComponent{X: 100, Y: 0, Z: 0})
	world.AddComponent(player2.ID, &VelocityComponent{X: -3, Y: 0, Z: 0})
	world.AddComponent(player2.ID, &HealthComponent{Current: 100, Max: 100})
	world.AddComponent(player2.ID, &PlayerComponent{PlayerID: "player_2"})

	// NPC erstellen
	npc := world.CreateEntity()
	world.AddComponent(npc.ID, &PositionComponent{X: 50, Y: 50, Z: 0})
	world.AddComponent(npc.ID, &HealthComponent{Current: 50, Max: 50})

	// Game Loop Simulation
	ticker := time.NewTicker(16 * time.Millisecond) // ~60 FPS
	defer ticker.Stop()

	frameCount := 0
	maxFrames := 60 // 1 Sekunde bei 60 FPS

	for range ticker.C {
		dt := 0.016 // 16ms

		// World Update
		world.Update(dt)

		// Network Sync - Dirty Entities an Clients senden
		dirtyEntities := world.GetDirtyEntities()
		if len(dirtyEntities) > 0 {
			// Hier würden die Daten an Clients gesendet werden
			println("Syncing", len(dirtyEntities), "entities to clients")
			world.ClearDirty()
		}

		// Alle Spieler abfragen
		playerQuery := world.Query().
			With(ComponentTypePlayer, ComponentTypePosition).
			Execute()

		for _, pid := range playerQuery {
			entity, _ := world.GetEntity(pid)
			pos, _ := entity.GetComponent(ComponentTypePosition)
			player, _ := entity.GetComponent(ComponentTypePlayer)

			position := pos.(*PositionComponent)
			playerComp := player.(*PlayerComponent)

			println(playerComp.PlayerID, "at position:", position.X, position.Y)
		}

		frameCount++
		if frameCount >= maxFrames {
			break
		}
	}

	println("Game simulation complete")
}

// Beispiel für Snapshot-Serialisierung für Netzwerk
type EntitySnapshot struct {
	ID         EntityID
	Components map[ComponentType]interface{}
}

func (w *World) CreateSnapshot(entityIDs []EntityID) []EntitySnapshot {
	snapshots := make([]EntitySnapshot, 0, len(entityIDs))

	for _, id := range entityIDs {
		entity, ok := w.GetEntity(id)
		if !ok {
			continue
		}

		snapshot := EntitySnapshot{
			ID:         id,
			Components: make(map[ComponentType]interface{}),
		}

		entity.mu.RLock()
		for ct, comp := range entity.Components {
			// Hier würde man die Komponenten serialisieren
			snapshot.Components[ct] = comp
		}
		entity.mu.RUnlock()

		snapshots = append(snapshots, snapshot)
	}

	return snapshots
}
