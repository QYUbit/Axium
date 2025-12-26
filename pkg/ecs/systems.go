package ecs

import (
	"reflect"
	"sync"
	"time"
)

type SystemTrigger int

const (
	OnStartup SystemTrigger = iota
	OnUpdate
)

type System func(ctx SystemContext)

type SystemContext struct {
	*World
	Dt       float64
	Commands *CommandBuffer
}

type Scheduler struct {
	initSystems []*SystemNode
	systems     []*SystemNode
	batches     [][]*SystemNode
}

type SystemNode struct {
	reads    map[ComponentID]struct{}
	writes   map[ComponentID]struct{}
	runner   System
	commands *CommandBuffer
}

func NewScheduler() *Scheduler {
	return &Scheduler{
		initSystems: make([]*SystemNode, 0),
		systems:     make([]*SystemNode, 0),
	}
}

type SystemConfig struct {
	System  System
	Trigger SystemTrigger
	Reads   []Component
	Writes  []Component
}

func (s *Scheduler) AddSystem(config SystemConfig) {
	node := &SystemNode{
		reads:    make(map[ComponentID]struct{}),
		writes:   make(map[ComponentID]struct{}),
		runner:   config.System,
		commands: &CommandBuffer{},
	}

	for _, c := range config.Reads {
		node.reads[c.Id()] = struct{}{}
	}
	for _, c := range config.Reads {
		node.writes[c.Id()] = struct{}{}
	}

	switch config.Trigger {
	case OnStartup:
		s.initSystems = append(s.initSystems, node)
	case OnUpdate:
		s.systems = append(s.systems, node)
	default:
		s.systems = append(s.systems, node)
	}
}

func (s *Scheduler) Compile() {
	s.batches = s.computeBatches(s.systems)
}

func (s *Scheduler) computeBatches(systems []*SystemNode) [][]*SystemNode {
	var batches [][]*SystemNode
	remaining := make([]*SystemNode, len(systems))
	copy(remaining, systems)

	for len(remaining) > 0 {
		var currentBatch []*SystemNode
		var nextRemaining []*SystemNode

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

func systemsConflict(a, b *SystemNode) bool {
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
		sys.runner(ctx)

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

	w.processCommands(commands)

	for _, msgStore := range w.messages {
		msgStore.swap()
	}
}

func (s *Scheduler) executeBatch(w *World, dt float64, batch []*SystemNode) {
	if len(batch) == 1 {
		ctx := SystemContext{
			World:    w,
			Dt:       dt,
			Commands: batch[0].commands,
		}
		batch[0].runner(ctx)
		return
	}

	var wg sync.WaitGroup
	wg.Add(len(batch))

	for _, sys := range batch {
		go func(node *SystemNode) {
			defer wg.Done()

			ctx := SystemContext{
				World:    w,
				Dt:       dt,
				Commands: node.commands,
			}
			node.runner(ctx)
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

func AddComponent[T any](cb *CommandBuffer, id EntityID, initial T) {
	t := reflect.TypeFor[T]()
	cb.commands = append(cb.commands, Command{
		Op:        AddComponentToEntity,
		EntityId:  id,
		Type:      t,
		Value:     initial, // ! Boxing
		Timestamp: time.Now().UnixNano(),
	})
}

func RemoveComponent[T any](cb *CommandBuffer, id EntityID) {
	t := reflect.TypeFor[T]()
	cb.commands = append(cb.commands, Command{
		Op:        RemoveComponentFromEntity,
		EntityId:  id,
		Type:      t,
		Timestamp: time.Now().UnixNano(),
	})
}
