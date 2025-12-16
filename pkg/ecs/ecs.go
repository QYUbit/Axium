package main

import (
	"fmt"
	"reflect"
	"slices"
	"sync"
	"time"
)

type EntityID uint64
type ComponentID uint16

type Component interface {
	Id() ComponentID
}

// ==================================================================
// World
// ==================================================================

type World struct {
	entities   map[EntityID]struct{}
	stores     map[reflect.Type]TypedStore
	singletons map[reflect.Type]any

	initSystems []*system
	systems     []*system
	batches     [][]*system
}

func NewWorld() *World {
	return &World{
		entities:   make(map[EntityID]struct{}),
		stores:     make(map[reflect.Type]TypedStore),
		singletons: make(map[reflect.Type]any),
	}
}

type Plugin func(w *World)

func (w *World) RegisterPlugin(plugin Plugin) {
	plugin(w)
}

// ==================================================================
// Commands
// ==================================================================

type CommandOperation int

const (
	CreateEntityCommand CommandOperation = iota
	DestroyEntityCommand
	AddComponentToEntity
	RemoveComponentFromEntity
)

type Command struct {
	Op        CommandOperation
	EntityId  EntityID
	Type      reflect.Type
	Value     any
	Timestamp int64
}

type CommandBuffer struct {
	commands []Command
}

func (cb *CommandBuffer) Reset() {
	cb.commands = cb.commands[:0]
}

func (cb *CommandBuffer) GetCommands() []Command {
	return cb.commands
}

func (cb *CommandBuffer) CreateEntity(id EntityID) {
	cb.commands = append(cb.commands, Command{
		Op:        CreateEntityCommand,
		EntityId:  id,
		Timestamp: time.Now().UnixNano(),
	})
}

func (cb *CommandBuffer) DestroyEntity(id EntityID) {
	cb.commands = append(cb.commands, Command{
		Op:        DestroyEntityCommand,
		EntityId:  id,
		Timestamp: time.Now().UnixNano(),
	})
}

func AddField[T any](cb *CommandBuffer, id EntityID, initial T) {
	t := reflect.TypeFor[T]()
	cb.commands = append(cb.commands, Command{
		Op:        AddComponentToEntity,
		EntityId:  id,
		Type:      t,
		Value:     initial, // ! Boxing
		Timestamp: time.Now().UnixNano(),
	})
}

func RemoveField[T any](cb *CommandBuffer, id EntityID) {
	t := reflect.TypeFor[T]()
	cb.commands = append(cb.commands, Command{
		Op:        RemoveComponentFromEntity,
		EntityId:  id,
		Type:      t,
		Timestamp: time.Now().UnixNano(),
	})
}

func (w *World) processCommands(commands []Command) {
	for _, cmd := range commands {
		switch cmd.Op {
		case CreateEntityCommand:
			w.createEntity(cmd.EntityId)
		case DestroyEntityCommand:
			w.destroyEntity(cmd.EntityId)
		case AddComponentToEntity:
			w.addField(cmd.EntityId, cmd.Type, cmd.Value)
		case RemoveComponentFromEntity:
			w.removeField(cmd.EntityId, cmd.Type)
		}
	}
}

func (w *World) createEntity(id EntityID) {
	w.entities[id] = struct{}{}
}

func (w *World) destroyEntity(id EntityID) {
	delete(w.entities, id)

	for _, store := range w.stores {
		if store.HasEntity(id) {
			store.Remove(id)
		}
	}
}

func (w *World) addField(id EntityID, typ reflect.Type, initial any) {
	s, ok := w.stores[typ]
	if ok {
		s.Add(id, initial)
	}
}

func (w *World) removeField(id EntityID, typ reflect.Type) {
	s, ok := w.stores[typ]
	if ok {
		s.Remove(id)
	}
}

// ==================================================================
// Systems
// ==================================================================

type TypeMask map[reflect.Type]struct{}

func NewTypeMask1[T any]() TypeMask {
	m := map[reflect.Type]struct{}{}
	m[reflect.TypeFor[T]()] = struct{}{}
	return m
}

func NewTypeMask2[T1, T2 any]() TypeMask {
	m := map[reflect.Type]struct{}{}
	m[reflect.TypeFor[T1]()] = struct{}{}
	m[reflect.TypeFor[T2]()] = struct{}{}
	return m
}

type SystemTrigger int

const (
	OnStartup SystemTrigger = iota
	OnUpdate
)

type SystemContext struct {
	*World
	Dt       float64
	Commands *CommandBuffer
}

type System func(ctx SystemContext)

type system struct {
	reads    map[ComponentID]struct{}
	writes   map[ComponentID]struct{}
	runner   func(ctx SystemContext)
	commands *CommandBuffer
}

func (w *World) RegisterSystem(sys System, trigger SystemTrigger, reads, writes []Component) {
	rs := map[ComponentID]struct{}{}
	ws := map[ComponentID]struct{}{}

	for _, c := range reads {
		rs[c.Id()] = struct{}{}
	}

	for _, c := range writes {
		ws[c.Id()] = struct{}{}
	}

	s := &system{
		reads:    rs,
		writes:   ws,
		runner:   sys,
		commands: &CommandBuffer{},
	}

	switch trigger {
	case OnUpdate:
		w.systems = append(w.systems, s)
	case OnStartup:
		w.initSystems = append(w.initSystems, s)
	}
}

func (w *World) Update(dt float64) {
	for _, batch := range w.batches {
		var wg sync.WaitGroup
		wg.Add(len(batch))

		for _, sys := range batch {
			go func(s *system) {
				defer wg.Done()

				ctx := SystemContext{
					World:    w,
					Dt:       dt,
					Commands: s.commands,
				}

				s.runner(ctx)
			}(sys)
		}

		wg.Wait()
	}

	var commands []Command

	for _, sys := range w.systems {
		commands = append(commands, sys.commands.GetCommands()...)
		sys.commands.Reset()
	}

	slices.SortFunc(commands, func(a, b Command) int {
		if a.Timestamp < b.Timestamp {
			return -1
		}
		if a.Timestamp > b.Timestamp {
			return 1
		}
		return 0
	})

	w.processCommands(commands)
}

func (w *World) Run() {
	for _, sys := range w.initSystems {
		ctx := SystemContext{
			World:    w,
			Commands: sys.commands,
		}

		sys.runner(ctx)

		w.processCommands(sys.commands.GetCommands())
		sys.commands.Reset()
	}

	w.batches = w.computeSystemBatches()
}

func (w *World) computeSystemBatches() [][]*system {
	var batches [][]*system
	remaining := make([]*system, len(w.systems))
	copy(remaining, w.systems)

	for len(remaining) > 0 {
		var currentBatch []*system
		var nextRemaining []*system

		for _, sys := range remaining {
			canRun := true

			for _, batchSys := range currentBatch {
				if systemsConflict(sys, batchSys) {
					canRun = false
					break
				}
			}

			if canRun {
				currentBatch = append(currentBatch, sys)
			} else {
				nextRemaining = append(nextRemaining, sys)
			}
		}

		if len(currentBatch) == 0 {
			currentBatch = append(currentBatch, remaining[0])
			nextRemaining = remaining[1:]
		}

		batches = append(batches, currentBatch)
		remaining = nextRemaining
	}

	return batches
}

func systemsConflict(a, b *system) bool {
	for id := range a.writes {
		if _, ok := b.writes[id]; ok {
			return true
		}
	}

	for id := range a.writes {
		if _, ok := b.reads[id]; ok {
			return true
		}
	}

	for id := range b.writes {
		if _, ok := a.reads[id]; ok {
			return true
		}
	}

	return false
}

// ==================================================================
// Singleton
// ==================================================================

type SingletonStore[T any] struct {
	data  T
	dirty bool
}

func RegisterSingleton[T Component](w *World, initial T) {
	t := reflect.TypeFor[T]()

	store := &SingletonStore[T]{
		data: initial,
	}

	w.singletons[t] = store
}

func GetSingleton[T any](w *World) *T {
	t := reflect.TypeFor[T]()
	s := w.singletons[t]

	store, ok := s.(*SingletonStore[T])
	if !ok {
		return nil
	}

	return &store.data
}

func GetMutableSingleton[T any](w *World) *T {
	t := reflect.TypeFor[T]()
	s := w.singletons[t]

	store, ok := s.(*SingletonStore[T])
	if !ok {
		return nil
	}

	store.dirty = true
	return &store.data
}

func GetStaticSingleton[T any](w *World) T {
	t := reflect.TypeFor[T]()
	s := w.singletons[t]

	store, ok := s.(*SingletonStore[T])
	if !ok {
		var zero T
		return zero
	}

	return store.data
}

// ==================================================================
// Store
// ==================================================================

func RegisterComponent[T Component](w *World) {
	t := reflect.TypeFor[T]()

	s := &Store[T]{
		typ:           t,
		entityToDense: make(map[EntityID]int),
	}

	w.stores[t] = s
}

func getStoreFromWorld[T any](w *World) (*Store[T], bool) {
	t := reflect.TypeFor[T]()

	s, ok := w.stores[t]
	if !ok {
		return nil, false
	}
	store, ok := s.(*Store[T])
	return store, ok
}

type TypedStore interface {
	GetEntities() []EntityID
	HasEntity(id EntityID) bool
	Add(id EntityID, value any)
	Remove(id EntityID)
}

type Store[T any] struct {
	typ           reflect.Type
	entityToDense map[EntityID]int
	denseToEntity []EntityID
	data          []T
	dirty         []bool
}

func (s *Store[T]) Add(id EntityID, value any) {
	if s.HasEntity(id) {
		return
	}

	initial, ok := value.(T)
	if !ok {
		return
	}

	newIndex := len(s.data)

	s.data = append(s.data, initial)
	s.denseToEntity = append(s.denseToEntity, id)

	s.dirty = append(s.dirty, false)

	s.entityToDense[id] = newIndex
}

func (s *Store[T]) Remove(id EntityID) {
	idx, exists := s.entityToDense[id]
	if !exists {
		return
	}

	lastIndex := len(s.data) - 1
	lastEntityID := s.denseToEntity[lastIndex]

	if idx != lastIndex {
		s.data[idx] = s.data[lastIndex]
		s.denseToEntity[idx] = lastEntityID
		s.dirty[idx] = s.dirty[lastIndex]

		s.entityToDense[lastEntityID] = idx
	}

	s.data = s.data[:lastIndex]
	s.denseToEntity = s.denseToEntity[:lastIndex]
	s.dirty = s.dirty[:lastIndex]

	delete(s.entityToDense, id)
}

func (s *Store[T]) GetEntities() []EntityID {
	return s.denseToEntity
}

func (s *Store[T]) HasEntity(id EntityID) bool {
	_, ok := s.entityToDense[id]
	return ok
}

func (s *Store[T]) Get(id EntityID) *T {
	return &s.data[s.entityToDense[id]]
}

func (s *Store[T]) GetMutable(id EntityID) *T {
	idx := s.entityToDense[id]
	s.dirty[idx] = true
	return &s.data[idx]
}

func (s *Store[T]) GetStatic(id EntityID) T {
	return s.data[s.entityToDense[id]]
}

// ==================================================================
// Example
// ==================================================================

type Position struct{ X, Y float64 }

func (Position) Id() ComponentID { return 0 }

type Velocity struct{ X, Y float64 }

func (Velocity) Id() ComponentID { return 1 }

type GameSpeed struct{ Speed float64 }

func (GameSpeed) Id() ComponentID { return 1001 }

func SetupSystem(ctx SystemContext) {
	player := EntityID(0)
	ctx.Commands.CreateEntity(player)

	AddField(ctx.Commands, player, Position{})
	AddField(ctx.Commands, player, Velocity{0, 1})
}

func MovementSystem(ctx SystemContext) {
	q := Query2[Position, Velocity](ctx.World)

	gameSpeed := GetSingleton[GameSpeed](ctx.World)

	for q.Next() {
		pos := q.GetMutable1()
		vel := q.Get2()

		pos.X += vel.X * ctx.Dt * gameSpeed.Speed
		pos.Y += vel.Y * ctx.Dt * gameSpeed.Speed
	}
}

func PrintPositionsSystem(ctx SystemContext) {
	q := Query1[Position](ctx.World)

	for q.Next() {
		e := q.GetEntity()
		pos := q.GetStatic()
		fmt.Println(e, pos)
	}
}

func MyGame(w *World) {
	RegisterComponent[Position](w)
	RegisterComponent[Velocity](w)

	RegisterSingleton[GameSpeed](w, GameSpeed{1})

	w.RegisterSystem(
		SetupSystem,
		OnStartup,
		nil,
		nil,
	)

	w.RegisterSystem(
		MovementSystem,
		OnUpdate,
		[]Component{Position{}, Velocity{}, GameSpeed{}},
		[]Component{Position{}},
	)

	w.RegisterSystem(
		PrintPositionsSystem,
		OnUpdate,
		[]Component{Position{}},
		nil,
	)
}

func main() {
	w := NewWorld()

	w.RegisterPlugin(MyGame)

	w.Run()

	w.Update(1)
}
