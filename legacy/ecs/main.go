package main

import (
	"fmt"
	"time"
)

const (
	PositionKey = "position"
	VelocityKey = "velocity"
)

type Position struct{ X, Y float64 }
type Velocity struct{ X, Y float64 }

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
		pos, ok1 := Get[Position](se, e, PositionKey)
		vel, ok2 := Get[Velocity](se, e, VelocityKey)

		if !ok1 || !ok2 {
			continue
		}

		pos.X += vel.X
		pos.Y += vel.Y

		se.MarkDirty(e, PositionKey)
		se.MarkDirty(e, VelocityKey)
	}
}

func main() {
	bus := NewEventBus()
	store := NewStore(nil)

	se := NewStateEngine(store, bus)

	RegisterComponent[Position](se, PositionKey)
	RegisterComponent[Velocity](se, VelocityKey)

	se.RegisterSystem(MovementSystem{})

	player := se.CreateEntityWithComponents(PositionKey, VelocityKey)

	vel := Velocity{2, 1}
	ok := Set(se, player, VelocityKey, &vel)
	if !ok {
		return
	}

	se.Update(time.Millisecond * 10)
	se.Update(time.Millisecond * 10)
	se.Update(time.Millisecond * 10)

	pos, ok := Get[Position](se, player, PositionKey)
	if !ok {
		return
	}

	fmt.Println(pos.X, pos.Y)
}
