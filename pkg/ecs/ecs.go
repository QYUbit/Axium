package ecs

import (
	"context"
	"reflect"
	"sync/atomic"
	"time"
)

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

type SystemOption func(*SystemConfig)

func Trigger(trigger SystemTrigger) SystemOption {
	return func(config *SystemConfig) {
		config.Trigger = trigger
	}
}

func Reads(comps ...any) SystemOption {
	return func(config *SystemConfig) {
		for _, comp := range comps {
			config.Reads[reflect.TypeOf(comp)] = struct{}{}
		}
	}
}

func Writes(comps ...any) SystemOption {
	return func(config *SystemConfig) {
		for _, comp := range comps {
			config.Writes[reflect.TypeOf(comp)] = struct{}{}
		}
	}
}

func (ecs *ECSEngine) RegisterSystem(sys System, opts ...SystemOption) {
	if !ecs.canRegister.Load() {
		panic("ECS engine has already started")
	}

	config := SystemConfig{
		System: sys,
		Reads:  make(map[reflect.Type]struct{}),
		Writes: make(map[reflect.Type]struct{}),
	}

	for _, opt := range opts {
		opt(&config)
	}

	ecs.scheduler.AddSystem(config)
}

func RegisterComponent[T Component](ecs *ECSEngine, id uint16) {
	if !ecs.canRegister.Load() {
		panic("ECS engine has already started")
	}
	registerComponent[T](ecs.world, false, id)
}

func RegisterComponentAuto[T Component](ecs *ECSEngine) {
	if !ecs.canRegister.Load() {
		panic("ECS engine has already started")
	}
	registerComponent[T](ecs.world, true, 0)
}

func RegisterSingleton[T Component](ecs *ECSEngine, initial T) {
	if !ecs.canRegister.Load() {
		panic("ECS engine has already started")
	}
	registerSingleton(ecs.world, initial)
}

func RegisterMessage[T Component](ecs *ECSEngine) {
	if !ecs.canRegister.Load() {
		panic("ECS engine has already started")
	}
	registerMessage[T](ecs.world)
}

// ==================================================================
// Global
// ==================================================================

func GetSingleton[T Component](w *World) *T {
	return getSingleton[T](w)
}

func GetMutableSingleton[T Component](w *World) *T {
	return getMutableSingleton[T](w)
}

func GetStaticSingleton[T Component](w *World) T {
	return getStaticSingleton[T](w)
}

func PushMessage[T Component](w *World, msg T) {
	pushMessage(w, msg)
}

func PushMessageSafe[T Component](ecs *ECSEngine, msg T) {
	pushMessageSafe(ecs.world, msg)
}

func CollectMessages[T Component](w *World) []T {
	return collectMessages[T](w)
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
}

func (ecs *ECSEngine) tick(dt float64) {
	ecs.scheduler.RunUpdate(ecs.world, dt)
}

func (ecs *ECSEngine) Wait() {
	<-ecs.done
}
