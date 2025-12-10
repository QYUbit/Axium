package main

import (
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
)

type System interface {
	Init()
	Update(ctx *WorldContext, dt float64)
	Name() string
}

type World struct {
	registry  *WorldRegistry
	store     *Store
	changeBuf *ChangeBuffer

	systems []System

	running atomic.Bool
}

func NewWorld() *World {
	return &World{
		store: newStore(),
	}
}

func RegisterComponent[T any](w *World, id ComponentID) error {
	if w.running.Load() {
		return errors.New("world is already running")
	}

	var zero T
	t := reflect.TypeOf(zero)

	w.store.components[id] = &ComponentInfo{
		typ:   t,
		size:  t.Size(),
		align: uintptr(t.Align()),
	}
	return nil
}

func (w *World) RegisterSystem(system System) error {
	if w.running.Load() {
		return errors.New("world is already running")
	}

	w.systems = append(w.systems, system)
	return nil
}

func (w *World) Run() error {
	if ok := w.running.CompareAndSwap(false, true); !ok {
		return errors.New("world is already running")
	}

	for _, system := range w.systems {
		system.Init()
	}

	return nil
}

func (w *World) Update(dt float64) error {
	if !w.running.Load() {
		return errors.New("world is not running yet")
	}

	view := w.store.TakeSnapshot()

	ctx := newWorldContext(w.registry, view, w.changeBuf)

	var wg sync.WaitGroup

	for _, system := range w.systems {
		wg.Add(1)
		go func() {
			defer wg.Done()
			system.Update(ctx, dt)
		}()
	}

	wg.Wait()

	w.store.ProcessChanges(w.changeBuf)

	w.changeBuf.clear()

	return nil
}

const (
	PositionType = 0
	VelocityType = 1
)

type Position struct{ X, Y float64 }
type Velocity struct{ X, Y float64 }

type MySystem struct{}

func (s MySystem) Name() string { return "my_system" }

func (s MySystem) Init() {}

func (s MySystem) Update(ctx *WorldContext, dt float64) {
	entities := ctx.Query(NewQuery().With(0, 1))

	for _, e := range entities {
		pos, _ := GetComponent[Position](ctx, e, PositionType)
		vel, _ := GetComponent[Velocity](ctx, e, VelocityType)

		pos.X += vel.X
		pos.Y += vel.Y

		Set(ctx, e, 0, pos)
	}
}

func main() {
	w := NewWorld()

	RegisterComponent[Position](w, PositionType)
	RegisterComponent[Velocity](w, VelocityType)

	w.RegisterSystem(MySystem{})

	w.Run()

	w.Update(0.1)
}
