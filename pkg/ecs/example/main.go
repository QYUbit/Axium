package main

import (
	"context"
	"fmt"
	"time"

	"github.com/QYUbit/Axium/pkg/ecs"
)

type Position struct{ X, Y float64 }
type Velocity struct{ X, Y float64 }

type GameSpeed struct{ Speed float64 }

type DeltaThingy struct {
	Entity ecs.Entity
}

func SetupSystem(ctx ecs.SystemContext) {
	player := ecs.Entity(0)
	ctx.Commands.CreateEntity(player, Position{}, Velocity{1, 0})
}

func MovementSystem(ctx ecs.SystemContext) {
	gameSpeed, _ := ecs.GetSingleton[GameSpeed](ctx.World)
	speed := gameSpeed.Get().Speed

	q := ecs.Query2[Position, Velocity](ctx.World)

	for row := range q.Iter() {
		pos := row.Mut1()
		vel := row.Get2()

		pos.X += vel.X * ctx.Dt * speed
		pos.Y += vel.Y * ctx.Dt * speed
	}
}

func PrintPositionsSystem(ctx ecs.SystemContext) {
	q := ecs.Query1[Position](ctx.World)

	for row := range q.Iter() {
		fmt.Printf("Entity %d at %v\n", row.E, *row.Get())
	}
}

func MyGame(engine *ecs.ECSEngine) {
	ecs.RegisterComponent[Position](engine, 0)
	ecs.RegisterComponent[Velocity](engine, 1)

	ecs.RegisterSingleton(engine, GameSpeed{1})

	engine.RegisterSystemFunc(
		SetupSystem,
		ecs.Trigger(ecs.OnStartup),
	)

	engine.RegisterSystemFunc(
		MovementSystem,
		ecs.Reads(Velocity{}, Position{}, GameSpeed{}),
		ecs.Writes(Position{}),
	)

	engine.RegisterSystemFunc(
		PrintPositionsSystem,
		ecs.Reads(Position{}),
	)
}

func main() {
	engine := ecs.NewEngine()

	engine.RegisterPlugin(MyGame)

	ctx := context.Background()

	go func(ctx context.Context) {
		engine.Run(ctx, 60)
	}(ctx)

	time.Sleep(time.Second * 3)

	engine.Close()
}
