package ecs

import (
	"context"
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

	phase atomic.Int32

	cancel context.CancelFunc
	done   chan struct{}
}

func NewEngine() *ECSEngine {
	return &ECSEngine{
		world:     NewWorld(),
		scheduler: NewScheduler(),
		done:      make(chan struct{}),
	}
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
	plugin(ecs)
}

type SystemOption func(*SystemConfig)

func Trigger(trigger SystemTrigger) SystemOption {
	return func(config *SystemConfig) {
		config.Trigger = trigger
	}
}

func Reads(comps ...Component) SystemOption {
	return func(config *SystemConfig) {
		config.Reads = append(config.Reads, comps...)
	}
}

func Writes(comps ...Component) SystemOption {
	return func(config *SystemConfig) {
		config.Writes = append(config.Writes, comps...)
	}
}

func (ecs *ECSEngine) RegisterSystem(sys System, opts ...SystemOption) {
	config := SystemConfig{
		System: sys,
	}

	for _, opt := range opts {
		opt(&config)
	}

	ecs.scheduler.AddSystem(config)
}

func RegisterComponent[T Component](ecs *ECSEngine) {
	registerComponent[T](ecs.world)
}

func RegisterSingleton[T Component](ecs *ECSEngine, initial T) {
	registerSingleton[T](ecs.world, initial)
}

func RegisterMessage[T Component](ecs *ECSEngine) {
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
	pushMessage[T](w, msg)
}

func PushMessageSafe[T Component](ecs *ECSEngine, msg T) {
	pushMessageSafe[T](ecs.world, msg)
}

func CollectMessages[T Component](w *World) []T {
	return collectMessages[T](w)
}

// ==================================================================
// Event Loop
// ==================================================================

func (ecs *ECSEngine) Run(ctx context.Context, tickRate int) {
	ecs.phase.Store(int32(Systems))

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
