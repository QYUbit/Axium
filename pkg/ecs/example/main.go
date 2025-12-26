package main

import (
	"context"
	"fmt"
	"time"

	"github.com/QYUbit/Axium/pkg/ecs"
)

type Position struct{ X, Y float64 }

func (Position) Id() ecs.ComponentID { return 0 }

type Velocity struct{ X, Y float64 }

func (Velocity) Id() ecs.ComponentID { return 1 }

type GameSpeed struct{ Speed float64 }

func (GameSpeed) Id() ecs.ComponentID { return 1001 }

func SetupSystem(ctx ecs.SystemContext) {
	player := ecs.EntityID(0)
	ctx.Commands.CreateEntity(player)

	ecs.AddComponent(ctx.Commands, player, Position{})
	ecs.AddComponent(ctx.Commands, player, Velocity{0, 1})
}

func MovementSystem(ctx ecs.SystemContext) {
	gameSpeed := ecs.GetSingleton[GameSpeed](ctx.World)
	q := ecs.SimpleQuery2[Position, Velocity](ctx.World)

	for row := range q {
		pos := row.GetMutable1()
		vel := row.Get2()

		pos.X += vel.X * ctx.Dt * gameSpeed.Speed
		pos.Y += vel.Y * ctx.Dt * gameSpeed.Speed
	}
}

func PrintPositionsSystem(ctx ecs.SystemContext) {
	q := ecs.SimpleQuery1[Position](ctx.World)

	for row := range q {
		pos := row.GetStatic()
		fmt.Printf("Entity %d at %v\n", row.ID, pos)
	}
}

func MyGame(engine *ecs.ECSEngine) {
	ecs.RegisterComponent[Position](engine)
	ecs.RegisterComponent[Velocity](engine)

	ecs.RegisterSingleton[GameSpeed](engine, GameSpeed{1})

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
