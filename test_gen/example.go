package main

type Position struct{ X, Y float64 }
type Velocity struct{ X, Y float64 }
type GameTime struct{ Delta float64 }

type MovementScope struct {
	Position Position `ecs:"read,write"`
	Velocity Velocity `ecs:"read"`
	GameTime GameTime `ecs:"singleton,read"`
}
