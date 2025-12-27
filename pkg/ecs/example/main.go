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

func SetupSystem(ctx ecs.SystemContext) {
	player := ecs.EntityID(0)
	ctx.Commands.CreateEntity(player)

	ecs.AddComponent(ctx.Commands, player, Position{})
	ecs.AddComponent(ctx.Commands, player, Velocity{0, 1})
}

func MovementSystem(ctx ecs.SystemContext) {
	gameSpeed := ecs.GetSingleton[GameSpeed](ctx.World)
	q := ecs.Query2[Position, Velocity](ctx.World)

	for row := range q.Iter() {
		pos := row.C1
		vel := row.C2

		pos.X += vel.X * ctx.Dt * gameSpeed.Speed
		pos.Y += vel.Y * ctx.Dt * gameSpeed.Speed
	}
}

func PrintPositionsSystem(ctx ecs.SystemContext) {
	q := ecs.Query1[Position](ctx.World)

	for row := range q.Iter() {
		fmt.Printf("Entity %d at %v\n", row.ID, row.C)
	}
}

func MyGame(engine *ecs.ECSEngine) {
	ecs.RegisterComponent[Position](engine, 0)
	ecs.RegisterComponent[Velocity](engine, 1)

	ecs.RegisterSingleton(engine, GameSpeed{1})

	engine.RegisterSystem(
		SetupSystem,
		ecs.Trigger(ecs.OnStartup),
	)

	engine.RegisterSystem(
		MovementSystem,
		ecs.Reads(Velocity{}, Position{}, GameSpeed{}),
		ecs.Writes(Position{}),
	)

	engine.RegisterSystem(
		PrintPositionsSystem,
		ecs.Reads(Position{}),
	)
}

func main() {
	engine := ecs.NewEngine()

	engine.RegisterPlugin(MyGame)

	ctx, cancel := context.WithCancel(context.Background())

	go func(ctx context.Context) {
		engine.Run(ctx, 60)
		engine.Wait()
	}(ctx)

	time.Sleep(time.Second * 3)

	cancel()
}
