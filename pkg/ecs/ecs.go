package ecs

import (
	"context"
	"sync/atomic"
	"time"
)

// ECSEngine represents an ECS.
type ECSEngine struct {
	world     *World
	scheduler *Scheduler

	canRegister atomic.Bool

	cancel context.CancelFunc
	done   chan struct{}
}

// NewEngine creates a new ECSEngine.
func NewEngine() *ECSEngine {
	ecs := &ECSEngine{
		world:     NewWorld(),
		scheduler: NewScheduler(),
		done:      make(chan struct{}),
	}
	ecs.canRegister.Store(true)
	return ecs
}

type EngineConfig struct {
	World     *World
	Scheduler *Scheduler
}

func NewEngineWithOptions(config EngineConfig) *ECSEngine {
	ecs := &ECSEngine{
		world:     config.World,
		scheduler: config.Scheduler,
	}

	if ecs.world == nil {
		ecs.world = NewWorld()
	}

	if ecs.scheduler == nil {
		ecs.scheduler = NewScheduler()
	}

	return ecs
}

func (ecs *ECSEngine) tryRegister() {
	if !ecs.canRegister.Load() {
		panic("ECS engine has already started")
	}
}

// ==================================================================
// Registration
// ==================================================================

// An ECSPlugin is a function which defines components, systems, etc.
// for an ECSEngine.
type ECSPlugin func(ecs *ECSEngine)

// RegisterPlugin registers an ECSPlugin.
// Calls while the engine is running will panic.
func (ecs *ECSEngine) RegisterPlugin(plugin ECSPlugin) {
	ecs.tryRegister()
	plugin(ecs)
}

// RegisterSystem registers the system sys using the optional options opts.
// Calls while the engine is running will panic.
func (ecs *ECSEngine) RegisterSystem(sys System, opts ...SystemOption) {
	ecs.tryRegister()
	ecs.scheduler.AddSystem(sys, opts)
}

// RegisterSystem registers the system sys using the optional options opts.
// Calls while the engine is running will panic.
func (ecs *ECSEngine) RegisterSystemFunc(sys SystemFunc, opts ...SystemOption) {
	ecs.tryRegister()
	ecs.scheduler.AddSystemFunc(sys, opts)
}

// RegisterComponent registers a new component store
// for type T with the identifier id.
// Calls while the engine is running will panic.
func RegisterComponent[T any](ecs *ECSEngine, id uint16) {
	ecs.tryRegister()
	registerComponent[T](ecs.world, false, id)
}

// RegisterComponent registers a new component store
// for type T with an automaticly generated identifier.
// Calls while the engine is running will panic.
func RegisterComponentAuto[T any](ecs *ECSEngine) {
	ecs.tryRegister()
	registerComponent[T](ecs.world, true, 0)
}

// RegisterSiglenton registers a new Singleton for T
// with the provided initial value.
// Calls while the engine is running will panic.
func RegisterSingleton[T any](ecs *ECSEngine, initial T) {
	ecs.tryRegister()
	registerSingleton(ecs.world, initial)
}

// RegisterMessage registers a message queue for message type T.
// Calls while the engine is running will panic.
func RegisterMessage[T any](ecs *ECSEngine) {
	ecs.tryRegister()
	registerMessage[T](ecs.world)
}

// ==================================================================
// Global
// ==================================================================

// Get retrieves the component T for the given entity e.
// Returns (nil, false) if the component doesn't exist.
// Not thread-safe.
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

// GetSingleton retrieves the singleton T.
// Returns (nil, false) if the sigleton doesn't exist.
// Not thread-safe.
func GetSingleton[T any](w *World) (*T, bool) {
	ptr := getSingleton[T](w)
	return ptr, ptr != nil
}

// PushMessage pushes msg to the message queue for T.
// Not thread-safe.
func PushMessage[T any](w *World, msg T) {
	pushMessage(w, msg)
}

// PushMessageSafe pushes msg to the message queue for T.
// Thread-safe for usage outside of systems.
func PushMessageSafe[T any](ecs *ECSEngine, msg T) {
	pushMessageSafe(ecs.world, msg)
}

// CollectMessages collects all messages form the message queue for T.
// Thread-safe for usage in systems.
func CollectMessages[T any](w *World) []T {
	return collectMessages[T](w)
}

// CollectMessagesSafe collects all messages form the message queue for T.
// Thread-safe.
func CollectMessagesSafe[T any](ecs *ECSEngine) []T {
	return collectMessagesSafe[T](ecs.world)
}

// ==================================================================
// Event Loop
// ==================================================================

// Run starts the event loop of the engine with the given tick rate.
// Will block until the context ctx is closed.
// Note: to stop the engine use Close().
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

func (ecs *ECSEngine) tick(dt float64) {
	ecs.scheduler.RunUpdate(ecs.world, dt)
}

// Close stops the event loop of the engine and waits until the
// current tick has been completed.
func (ecs *ECSEngine) Close() {
	ecs.cancel()
	<-ecs.done
}
