package main

import (
	"fmt"
	"time"
)

type BaseSystem struct {
	name  string
	stage int
}

func NewBaseSystem(name string, stage int) BaseSystem {
	return BaseSystem{name, stage}
}

func (bs BaseSystem) Name() string { return bs.name }

func (bs BaseSystem) Stage() int { return bs.stage }

func (bs BaseSystem) Init(_ *StateEngine) {}

func (bs BaseSystem) Update(_ *StateEngine, _ time.Duration) {}

type MovementSystem struct {
	BaseSystem
}

func NewMovementSystem() MovementSystem {
	return MovementSystem{NewBaseSystem("movement", 0)}
}

func (s MovementSystem) Update(se *StateEngine, dt time.Duration) {
	q := NewQuery().With(1, 2)
	entities := q.Execute(se)

	for _, e := range entities {
		pos, ok1 := Get[Position](se.store, e, 1)
		vel, ok2 := Get[Velocity](se.store, e, 2)

		if !ok1 || !ok2 {
			continue
		}

		pos.X = pos.X + vel.X
		pos.Y = pos.Y + vel.Y
	}
}

type Position struct{ X, Y float64 }
type Velocity struct{ X, Y float64 }

func main() {
	bus := NewEventBus()
	store := NewStore(nil)

	se := NewStateEngine(store, bus)

	posID := RegisterComponent[Position](se.store)
	velID := RegisterComponent[Velocity](se.store)

	se.RegisterSystem(MovementSystem{})

	player := se.store.CreateEntityWithComponents([]ComponentID{posID, velID})

	vel := Velocity{2, 1}
	ok := Set(se.store, player, velID, &vel)
	if !ok {
		return
	}

	se.Update(time.Millisecond * 10)
	se.Update(time.Millisecond * 10)
	se.Update(time.Millisecond * 10)

	pos, ok := Get[Position](se.store, player, posID)
	if !ok {
		return
	}

	fmt.Println(pos.X, pos.Y)
}
