package lib

import "github.com/QYUbit/Axium/pkg/ecs"

type Transform struct {
	X, Y, Z                         float64
	RotationX, RotationY, RotationZ float64
	ScaleX, ScaleY, ScaleZ          float64
}

func (Transform) Id() ecs.ComponentID { return 1 }

type Transform2D struct {
	X, Y           float64
	Rotation       float64 // Radians
	ScaleX, ScaleY float64
}

func (Transform2D) Id() ecs.ComponentID { return 2 }

type Position struct {
	X, Y, Z float64
}

func (Position) Id() ecs.ComponentID { return 3 }

type Position2D struct {
	X, Y float64
}

func (Position2D) Id() ecs.ComponentID { return 4 }

type Rotation struct {
	X, Y, Z float64 // Euler angles in radians
}

func (Rotation) Id() ecs.ComponentID { return 5 }

type Rotation2D struct {
	Angle float64 // Radians
}

func (Rotation2D) Id() ecs.ComponentID { return 6 }

type Velocity struct {
	X, Y, Z float64
}

func (Velocity) Id() ecs.ComponentID { return 7 }

type Velocity2D struct {
	X, Y float64
}

func (Velocity2D) Id() ecs.ComponentID { return 8 }

type Acceleration struct {
	X, Y, Z float64
}

func (Acceleration) Id() ecs.ComponentID { return 9 }

type Acceleration2D struct {
	X, Y float64
}

func (Acceleration2D) Id() ecs.ComponentID { return 10 }

type AngularVelocity struct {
	X, Y, Z float64
}

func (AngularVelocity) Id() ecs.ComponentID { return 11 }

type AngularVelocity2D struct {
	Value float64 // Radians per second
}

func (AngularVelocity2D) Id() ecs.ComponentID { return 12 }
