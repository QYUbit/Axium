package ecs

import (
	"context"
	"sync/atomic"
	"time"
)

// TODO Phases

type ECSPhase int

const (
	Registration ECSPhase = iota
	Systems
	Managed
)

type ECSEngine struct {
	world     *World
	scheduler *Scheduler

	canRegister atomic.Bool

	cancel context.CancelFunc
	done   chan struct{}
}

func NewEngine() *ECSEngine {
	ecs := &ECSEngine{
		world:     NewWorld(),
		scheduler: NewScheduler(),
		done:      make(chan struct{}),
	}
	ecs.canRegister.Store(true)
	return ecs
}

type EngineOptions struct {
	world     *World
	scheduler *Scheduler
}

func NewEngineWithOptions(options EngineOptions) *ECSEngine {
	ecs := &ECSEngine{
		world:     options.world,
		scheduler: options.scheduler,
	}

	if ecs.world == nil {
		ecs.world = NewWorld()
	}

	if ecs.scheduler == nil {
		ecs.scheduler = NewScheduler()
	}

	return ecs
}

// ==================================================================
// Registration
// ==================================================================

type ECSPlugin func(ecs *ECSEngine)

func (ecs *ECSEngine) RegisterPlugin(plugin ECSPlugin) {
	if !ecs.canRegister.Load() {
		panic("ECS engine has already started")
	}
	plugin(ecs)
}

func (ecs *ECSEngine) RegisterSystem(sys System, opts ...SystemOption) {
	if !ecs.canRegister.Load() {
		panic("ECS engine has already started")
	}
	ecs.scheduler.AddSystem(sys, opts)
}

func RegisterComponent[T any](ecs *ECSEngine, id uint16) {
	if !ecs.canRegister.Load() {
		panic("ECS engine has already started")
	}
	registerComponent[T](ecs.world, false, id)
}

func RegisterComponentAuto[T any](ecs *ECSEngine) {
	if !ecs.canRegister.Load() {
		panic("ECS engine has already started")
	}
	registerComponent[T](ecs.world, true, 0)
}

func RegisterSingleton[T any](ecs *ECSEngine, initial T) {
	if !ecs.canRegister.Load() {
		panic("ECS engine has already started")
	}
	registerSingleton(ecs.world, initial)
}

func RegisterMessage[T any](ecs *ECSEngine) {
	if !ecs.canRegister.Load() {
		panic("ECS engine has already started")
	}
	registerMessage[T](ecs.world)
}

// ==================================================================
// Global
// ==================================================================

func Get[T any](w *World, e Entity) (*T, bool) {
	s, ok := getStoreFromWorld[T](w)
	if !ok {
		return nil, false
	}
	ptr := s.Get(e)
	if ptr == nil {
		return nil, false
	}
	return ptr, true
}

func GetSingleton[T any](w *World) *T {
	return getSingleton[T](w)
}

func PushMessage[T any](w *World, msg T) {
	pushMessage(w, msg)
}

func PushMessageSafe[T any](ecs *ECSEngine, msg T) {
	pushMessageSafe(ecs.world, msg)
}

func CollectMessages[T any](w *World) []T {
	return collectMessages[T](w)
}

func CollectMessagesSafe[T any](w *World) []T {
	return collectMessagesSafe[T](w)
}

// ==================================================================
// Event Loop
// ==================================================================

func (ecs *ECSEngine) Run(ctx context.Context, tickRate int) {
	ecs.canRegister.Store(false)

	ctx, cancel := context.WithCancel(ctx)
	ecs.cancel = cancel

	ecs.scheduler.Compile()
	ecs.scheduler.RunInit(ecs.world)

	ticker := time.NewTicker(time.Second / time.Duration(tickRate))

	defer close(ecs.done)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ecs.tick(1.0 / float64(tickRate))
		}
	}
}

func (ecs *ECSEngine) Close() {
	ecs.cancel()
	<-ecs.done
}

func (ecs *ECSEngine) tick(dt float64) {
	ecs.scheduler.RunUpdate(ecs.world, dt)
}
