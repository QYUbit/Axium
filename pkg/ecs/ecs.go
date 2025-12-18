package ecs

import (
	"context"
	"time"
)

type ECS struct {
	world     *World
	scheduler *Scheduler

	done chan struct{}
}

func NewECS() *ECS {
	return &ECS{
		world:     NewWorld(),
		scheduler: NewScheduler(),
		done:      make(chan struct{}),
	}
}

type ECSOptions struct {
	world     *World
	scheduler *Scheduler
}

func NewECSWithOptions(options ECSOptions) *ECS {
	ecs := &ECS{
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

type ECSPlugin func(ecs *ECS)

func (ecs *ECS) RegisterPlugin(plugin ECSPlugin) {
	plugin(ecs)
}

func (ecs *ECS) RegisterSystem(sys System, trigger SystemTrigger, reads, writes []Component) {
	ecs.scheduler.AddSystem(sys, trigger, reads, writes)
}

func RegisterComp[T Component](ecs *ECS) {
	RegisterComponent[T](ecs.world)
}

func RegisterSing[T Component](ecs *ECS, initial T) {
	RegisterSingleton[T](ecs.world, initial)
}

func RegisterMess[T Component](ecs *ECS) {
	RegisterMessage[T](ecs.world)
}

// ==================================================================
// Event Loop
// ==================================================================

func (ecs *ECS) Run(ctx context.Context, tickRate int) {
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

func (ecs *ECS) tick(dt float64) {
	ecs.scheduler.RunUpdate(ecs.world, dt)
}

func (ecs *ECS) Done() {
	<-ecs.done
}
