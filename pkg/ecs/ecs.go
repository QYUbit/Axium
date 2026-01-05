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
	engine := &ECSEngine{
		world:     NewWorld(),
		scheduler: NewScheduler(),
		done:      make(chan struct{}),
	}
	engine.canRegister.Store(true)
	return engine
}

type EngineConfig struct {
	World     *World
	Scheduler *Scheduler
}

func NewEngineWithOptions(config EngineConfig) *ECSEngine {
	engine := &ECSEngine{
		world:     config.World,
		scheduler: config.Scheduler,
	}

	if engine.world == nil {
		engine.world = NewWorld()
	}

	if engine.scheduler == nil {
		engine.scheduler = NewScheduler()
	}

	return engine
}

func (engine *ECSEngine) tryRegister() {
	if !engine.canRegister.Load() {
		panic("ECS engine has already started")
	}
}

// ==================================================================
// Registration
// ==================================================================

// An ECSPlugin is a function which defines components, systems, etc.
// for an ECSEngine.
type ECSPlugin func(engine *ECSEngine)

// RegisterPlugin registers an ECSPlugin.
// Calls while the engine is running will panic.
func (engine *ECSEngine) RegisterPlugin(plugin ECSPlugin) {
	engine.tryRegister()
	plugin(engine)
}

// RegisterSystem registers the system sys using the optional options opts.
// Calls while the engine is running will panic.
func (engine *ECSEngine) RegisterSystem(sys System, opts ...SystemOption) {
	engine.tryRegister()
	engine.scheduler.AddSystem(sys, opts)
}

// RegisterSystem registers the system sys using the optional options opts.
// Calls while the engine is running will panic.
func (engine *ECSEngine) RegisterSystemFunc(sys SystemFunc, opts ...SystemOption) {
	engine.tryRegister()
	engine.scheduler.AddSystem(sys, opts)
}

// RegisterComponent registers a new component store
// for type T with the identifier id.
// Calls while the engine is running will panic.
func RegisterComponent[T any](engine *ECSEngine, id uint16) {
	engine.tryRegister()
	registerComponent[T](engine.world, false, id)
}

// RegisterComponent registers a new component store
// for type T with an automaticly generated identifier.
// Calls while the engine is running will panic.
func RegisterComponentAuto[T any](engine *ECSEngine) {
	engine.tryRegister()
	registerComponent[T](engine.world, true, 0)
}

// RegisterSiglenton registers a new Singleton for T
// with the provided initial value.
// Calls while the engine is running will panic.
func RegisterSingleton[T any](engine *ECSEngine, initial T) {
	engine.tryRegister()
	registerSingleton(engine.world, initial)
}

// RegisterMessage registers a message queue for message type T.
// Calls while the engine is running will panic.
func RegisterMessage[T any](engine *ECSEngine) {
	engine.tryRegister()
	registerMessage[T](engine.world)
}

// ==================================================================
// Global
// ==================================================================

// EntityExists reports whether entity e is present in w.
func (w *World) EntityExists(e Entity) bool {
	return w.entityExists(e)
}

// Get retrieves the component T for the given entity e.
// Returns (nil, false) if the component doesn't exist.
// Not thread-safe.
func Get[T any](w *World, e Entity) (*T, bool) {
	s, ok := getStoreFromWorld[T](w)
	if !ok {
		return nil, false
	}
	ptr := s.get(e)
	if ptr == nil {
		return nil, false
	}
	return ptr, true
}

// Get retrieves the component T for the given entity e.
// and marks it as dirty. Returns (nil, false) if the
// component doesn't exist. Not thread-safe.
func GetMut[T any](w *World, e Entity) (*T, bool) {
	s, ok := getStoreFromWorld[T](w)
	if !ok {
		return nil, false
	}
	ptr := s.mut(e)
	if ptr == nil {
		return nil, false
	}
	return ptr, true
}

// GetSingleton retrieves a wrapper for the singleton T.
// Returns (nil, false) if the sigleton doesn't exist.
// Not thread-safe.
func GetSingleton[T any](w *World) (*Singleton[T], bool) {
	s, ok := getSingleton[T](w)
	return s, ok
}

// PushMessage pushes msg to the message queue for T.
// Not thread-safe.
func PushMessage[T any](w *World, msg T) {
	pushMessage(w, msg)
}

// PushMessageSafe pushes msg to the message queue for T.
// Thread-safe for usage outside of systems.
func PushMessageSafe[T any](engine *ECSEngine, msg T) {
	pushMessageSafe(engine.world, msg)
}

// CollectMessages collects all messages form the message queue for T.
// Thread-safe for usage in systems.
func CollectMessages[T any](w *World) []T {
	return collectMessages[T](w)
}

// CollectMessagesSafe collects all messages form the message queue for T.
// Thread-safe.
func CollectMessagesSafe[T any](engine *ECSEngine) []T {
	return collectMessagesSafe[T](engine.world)
}

// ==================================================================
// Event Loop
// ==================================================================

// Run starts the event loop of the engine with the given tick rate.
// Will block until the context ctx is closed.
// Note: to stop the engine use Close().
func (engine *ECSEngine) Run(ctx context.Context, tickRate int) {
	engine.canRegister.Store(false)

	ctx, cancel := context.WithCancel(ctx)
	engine.cancel = cancel

	engine.scheduler.Compile()
	engine.scheduler.RunInit(engine.world)

	ticker := time.NewTicker(time.Second / time.Duration(tickRate))

	defer close(engine.done)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			engine.tick(1.0 / float64(tickRate))
		}
	}
}

func (engine *ECSEngine) tick(dt float64) {
	engine.scheduler.RunUpdate(engine.world, dt)
}

// Close stops the event loop of the engine and waits until the
// current tick has been completed.
func (engine *ECSEngine) Close() {
	engine.cancel()
	<-engine.done
}
