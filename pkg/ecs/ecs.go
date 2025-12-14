package main

import (
	"fmt"
	"reflect"
	"slices"
	"sync"
	"time"
)

type EntityID uint32

// ==================================================================
// World
// ==================================================================

type World struct {
	entities map[EntityID]struct{}
	stores   map[reflect.Type]TypedStore

	initSystems []*system
	systems     []*system
	batches     [][]*system
}

func NewWorld() *World {
	return &World{
		entities: make(map[EntityID]struct{}),
		stores:   make(map[reflect.Type]TypedStore),
	}
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

func RegisterType[T any](w *World) {
	t := reflect.TypeFor[T]()

	s := &Store[T]{
		typ:           t,
		entityToDense: make(map[EntityID]int),
	}

	w.stores[t] = s
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
		Value:     initial,
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
	reads    TypeMask
	writes   TypeMask
	runner   func(ctx SystemContext)
	commands *CommandBuffer
}

func (w *World) RegisterSystem(sys System, trigger SystemTrigger, reads, writes TypeMask) {
	if reads == nil {
		reads = map[reflect.Type]struct{}{}
	}
	if writes == nil {
		writes = map[reflect.Type]struct{}{}
	}

	s := &system{
		reads:    reads,
		writes:   writes,
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
// Store
// ==================================================================

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
	Data          []T
	ChangeTicks   []uint64
}

func (s *Store[T]) Add(id EntityID, value any) {
	if s.HasEntity(id) {
		return
	}

	initial, ok := value.(T)
	if !ok {
		return
	}

	newIndex := len(s.Data)

	s.Data = append(s.Data, initial)
	s.denseToEntity = append(s.denseToEntity, id)

	s.ChangeTicks = append(s.ChangeTicks, 0)

	s.entityToDense[id] = newIndex
}

func (s *Store[T]) Remove(id EntityID) {
	idx, exists := s.entityToDense[id]
	if !exists {
		return
	}

	lastIndex := len(s.Data) - 1
	lastEntityID := s.denseToEntity[lastIndex]

	if idx != lastIndex {
		s.Data[idx] = s.Data[lastIndex]
		s.denseToEntity[idx] = lastEntityID
		s.ChangeTicks[idx] = s.ChangeTicks[lastIndex]

		s.entityToDense[lastEntityID] = idx
	}

	s.Data = s.Data[:lastIndex]
	s.denseToEntity = s.denseToEntity[:lastIndex]
	s.ChangeTicks = s.ChangeTicks[:lastIndex]

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
	return &s.Data[s.entityToDense[id]]
}

func (s *Store[T]) GetMutable(id EntityID) *T {
	idx := s.entityToDense[id]
	s.ChangeTicks[idx]++
	return &s.Data[idx]
}

func (s *Store[T]) GetStatic(id EntityID) T {
	return s.Data[s.entityToDense[id]]
}

// ==================================================================
// Example
// ==================================================================

type Position struct{ X, Y float64 }
type Velocity struct{ X, Y float64 }

func SetupSystem(ctx SystemContext) {
	player := EntityID(0)
	ctx.Commands.CreateEntity(player)

	AddField(ctx.Commands, player, Position{})
	AddField(ctx.Commands, player, Velocity{0, 1})
}

func MovementSystem(ctx SystemContext) {
	q := Query2[Position, Velocity](ctx.World)

	for q.Next() {
		pos := q.GetMutable1()
		vel := q.Get2()

		pos.X += vel.X * ctx.Dt
		pos.Y += vel.Y * ctx.Dt
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

func main() {
	w := NewWorld()

	RegisterType[Position](w)
	RegisterType[Velocity](w)

	w.RegisterSystem(
		SetupSystem,
		OnStartup,
		nil,
		nil,
	)

	w.RegisterSystem(
		MovementSystem,
		OnUpdate,
		NewTypeMask2[Position, Velocity](),
		NewTypeMask1[Position](),
	)

	w.RegisterSystem(
		PrintPositionsSystem,
		OnUpdate,
		NewTypeMask1[Position](),
		nil,
	)

	w.Run()

	w.Update(1)
}
