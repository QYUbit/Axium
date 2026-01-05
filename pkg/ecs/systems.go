package ecs

import (
	"reflect"
	"sync"
	"time"
)

// TODO Improve Scheduler

type SystemTrigger int

const (
	// Executes one time at the start of the event loop. Executed sequentially.
	OnStartup SystemTrigger = iota
	// Executes every tick. Can be executed in parallel.
	OnUpdate
	// Executes at the and of every tick. Executed sequentially.
	// All command buffers are merged into one.
	OnEndOfTick
)

// SystemFunc is a function that can interact with the world state.
type SystemFunc func(ctx SystemContext)

func (f SystemFunc) Run(ctx SystemContext) {
	f(ctx)
}

// System has a method Run that can interact with the world state.
type System interface {
	Run(ctx SystemContext)
}

type SystemContext struct {
	*World
	Dt       float64
	Commands *CommandBuffer
}

type Scheduler struct {
	initSystems []*systemNode
	systems     []*systemNode
	batches     [][]*systemNode
	endSystems  []*systemNode
}

type systemNode struct {
	reads    map[reflect.Type]struct{}
	writes   map[reflect.Type]struct{}
	runner   System
	commands *CommandBuffer
}

// NewScheduler creates a new Scheduler.
func NewScheduler() *Scheduler {
	return &Scheduler{
		initSystems: make([]*systemNode, 0),
		systems:     make([]*systemNode, 0),
		endSystems:  make([]*systemNode, 0),
	}
}

// SystemConfig represents system configuration.
type SystemConfig struct {
	Trigger SystemTrigger
	Reads   map[reflect.Type]struct{}
	Writes  map[reflect.Type]struct{}
}

func buildSystemConfig(opts []SystemOption) SystemConfig {
	config := SystemConfig{
		Reads:   make(map[reflect.Type]struct{}),
		Writes:  make(map[reflect.Type]struct{}),
		Trigger: -1,
	}

	for _, opt := range opts {
		opt(&config)
	}

	return config
}

// System is a function that mutates SystemConfig. It provides a practical
// way to define system configuration.
type SystemOption func(*SystemConfig)

// Trigger returns a SystemOption. It specifies when a system will be executed.
func Trigger(trigger SystemTrigger) SystemOption {
	return func(config *SystemConfig) {
		config.Trigger = trigger
	}
}

// Read returns a SystemOption. It specifies which components and singletons
// a system will a read. It is used to determine which systems can run in parallel.
func Reads(comps ...any) SystemOption {
	return func(config *SystemConfig) {
		for _, comp := range comps {
			config.Reads[reflect.TypeOf(comp)] = struct{}{}
		}
	}
}

// Write returns a SystemOption. It specifies which components, singletons and message queues
// a system will a write. It is used to determine which systems can run in parallel.
func Writes(comps ...any) SystemOption {
	return func(config *SystemConfig) {
		for _, comp := range comps {
			config.Writes[reflect.TypeOf(comp)] = struct{}{}
		}
	}
}

func (s *Scheduler) AddSystem(sys System, opts []SystemOption) {
	config := buildSystemConfig(opts)

	node := &systemNode{
		reads:    config.Reads,
		writes:   config.Writes,
		runner:   sys,
		commands: &CommandBuffer{},
	}

	switch config.Trigger {
	case OnStartup:
		s.initSystems = append(s.initSystems, node)
	case OnEndOfTick:
		s.endSystems = append(s.endSystems, node)
	case OnUpdate:
		s.systems = append(s.systems, node)
	default:
		s.systems = append(s.systems, node)
	}
}

func (s *Scheduler) Compile() {
	s.batches = s.computeBatches(s.systems)
}

func (s *Scheduler) computeBatches(systems []*systemNode) [][]*systemNode {
	var batches [][]*systemNode
	remaining := make([]*systemNode, len(systems))
	copy(remaining, systems)

	for len(remaining) > 0 {
		var currentBatch []*systemNode
		var nextRemaining []*systemNode

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

func systemsConflict(a, b *systemNode) bool {
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

func (s *Scheduler) RunInit(w *World) {
	for _, sys := range s.initSystems {
		ctx := SystemContext{
			World:    w,
			Commands: sys.commands,
		}
		sys.runner.Run(ctx)

		w.processCommands(sys.commands.GetCommands())
		sys.commands.Reset()
	}
}

func (s *Scheduler) RunUpdate(w *World, dt float64) {
	for _, batch := range s.batches {
		s.executeBatch(w, dt, batch)
	}

	var commands []Command
	for _, sys := range s.systems {
		commands = append(commands, sys.commands.GetCommands()...)
		sys.commands.Reset()
	}

	for _, sys := range s.endSystems {
		sys.runner.Run(SystemContext{
			World: w,
			Dt:    dt,
			Commands: &CommandBuffer{
				commands: commands,
			},
		})
	}

	w.processCommands(commands)

	for _, msgStore := range w.messages {
		msgStore.swap()
	}
}

func (s *Scheduler) executeBatch(w *World, dt float64, batch []*systemNode) {
	if len(batch) == 1 {
		ctx := SystemContext{
			World:    w,
			Dt:       dt,
			Commands: batch[0].commands,
		}

		batch[0].runner.Run(ctx)
		return
	}

	var wg sync.WaitGroup
	wg.Add(len(batch))

	for _, sys := range batch {
		go func(node *systemNode) {
			defer wg.Done()

			ctx := SystemContext{
				World:    w,
				Dt:       dt,
				Commands: node.commands,
			}
			node.runner.Run(ctx)
		}(sys)
	}

	wg.Wait()
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

// Command represents a structural change instruction
// (create entity, destroy entity, add component, remove component).
type Command struct {
	Op        CommandOperation
	Entity    Entity
	Type      reflect.Type
	Value     any
	Values    map[reflect.Type]any
	Timestamp int64
}

// A CommandBuffer collects commands from systems and executes
// them at the end of the tick.
type CommandBuffer struct {
	commands []Command
}

// Reset resets the command buffer.
func (cb *CommandBuffer) Reset() {
	cb.commands = cb.commands[:0]
}

// GetCommands retrieves all queued commands from cb.
func (cb *CommandBuffer) GetCommands() []Command {
	return cb.commands
}

// TODO Auto entities

// CreateEntity inserts a create-entity-command to cb.
func (cb *CommandBuffer) CreateEntity(e Entity, initial ...any) {
	values := make(map[reflect.Type]any)
	for _, v := range initial {
		values[reflect.TypeOf(v)] = v
	}

	cb.commands = append(cb.commands, Command{
		Op:        CreateEntityCommand,
		Entity:    e,
		Values:    values,
		Timestamp: time.Now().UnixNano(),
	})
}

// DestroyEntity inserts a destroy-entity-command to cb.
func (cb *CommandBuffer) DestroyEntity(e Entity) {
	cb.commands = append(cb.commands, Command{
		Op:        DestroyEntityCommand,
		Entity:    e,
		Timestamp: time.Now().UnixNano(),
	})
}

// AddComponent inserts a add-component-command to cb.
func (cb *CommandBuffer) AddComponent(e Entity, v any) {
	// ! Boxing
	t := reflect.TypeOf(v)
	cb.commands = append(cb.commands, Command{
		Op:        AddComponentToEntity,
		Entity:    e,
		Type:      t,
		Value:     v,
		Timestamp: time.Now().UnixNano(),
	})
}

// RemoveComponent inserts a remove-component-command to cb.
func (cb *CommandBuffer) RemoveComponent(e Entity, v any) {
	t := reflect.TypeOf(v)
	cb.commands = append(cb.commands, Command{
		Op:        RemoveComponentFromEntity,
		Entity:    e,
		Type:      t,
		Timestamp: time.Now().UnixNano(),
	})
}

// RemoveComponentFor inserts a remove-component-command to cb.
func RemoveComponentFor[T any](cb *CommandBuffer, e Entity) {
	t := reflect.TypeFor[T]()
	cb.commands = append(cb.commands, Command{
		Op:        RemoveComponentFromEntity,
		Entity:    e,
		Type:      t,
		Timestamp: time.Now().UnixNano(),
	})
}
