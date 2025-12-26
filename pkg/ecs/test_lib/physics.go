package lib

import "github.com/QYUbit/Axium/pkg/ecs"

type PhysicsConfig struct {
	Gravity             float64
	FixedTimestep       float64
	MaxVelocity         float64
	CollisionIterations int
}

func (PhysicsConfig) Id() ecs.ComponentID { return 1000 }

type PhysicsConfig2D struct {
	Gravity             float64
	FixedTimestep       float64
	MaxVelocity         float64
	CollisionIterations int
}

func (PhysicsConfig2D) Id() ecs.ComponentID { return 1001 }

type RigidBody struct {
	Mass            float64
	Drag            float64
	AngularDrag     float64
	UseGravity      bool
	IsKinematic     bool
	FreezePositionX bool
	FreezePositionY bool
	FreezePositionZ bool
	Restitution     float64 // Bounciness (0-1)
	Friction        float64 // Surface friction (0-1)
}

func (RigidBody) Id() ecs.ComponentID { return 20 }

type RigidBody2D struct {
	Mass            float64
	Drag            float64
	AngularDrag     float64
	UseGravity      bool
	IsKinematic     bool
	FreezePositionX bool
	FreezePositionY bool
	FreezeRotation  bool
	Restitution     float64
	Friction        float64
}

func (RigidBody2D) Id() ecs.ComponentID { return 21 }

type Collider struct {
	Type      ColliderType
	Radius    float64 // For sphere/capsule
	Width     float64 // For box
	Height    float64 // For box/capsule
	Depth     float64 // For box
	IsTrigger bool
	Layer     uint32
	Offset    Position
}

type ColliderType int

const (
	ColliderBox ColliderType = iota
	ColliderSphere
	ColliderCapsule
)

func (Collider) Id() ecs.ComponentID { return 22 }

type Collider2D struct {
	Type      Collider2DType
	Radius    float64 // For circle
	Width     float64 // For box
	Height    float64 // For box/capsule
	IsTrigger bool
	Layer     uint32
	Offset    Position2D
}

type Collider2DType int

const (
	Collider2DBox Collider2DType = iota
	Collider2DCircle
	Collider2DCapsule
	Collider2DPolygon
)

func (Collider2D) Id() ecs.ComponentID { return 23 }

type CollisionEvent struct {
	EntityA   ecs.EntityID
	EntityB   ecs.EntityID
	Point     Position
	Normal    struct{ X, Y, Z float64 }
	Impulse   float64
	Timestamp int64
}

func (CollisionEvent) Id() ecs.ComponentID { return 24 }

type CollisionEvent2D struct {
	EntityA   ecs.EntityID
	EntityB   ecs.EntityID
	Point     Position2D
	Normal    struct{ X, Y float64 }
	Impulse   float64
	Timestamp int64
}

func (CollisionEvent2D) Id() ecs.ComponentID { return 25 }

func MovementSystem(ctx ecs.SystemContext) {
	q := ecs.SimpleQuery2[Position, Velocity](ctx.World)

	for row := range q {
		pos := row.GetMutable1()
		vel := row.Get2()

		pos.X += vel.X * ctx.Dt
		pos.Y += vel.Y * ctx.Dt
		pos.Z += vel.Z * ctx.Dt
	}
}

// Movement System 2D - Applies velocity to position
func MovementSystem2D(ctx ecs.SystemContext) {
	q := ecs.SimpleQuery2[Position2D, Velocity2D](ctx.World)

	for row := range q {
		pos := row.GetMutable1()
		vel := row.Get2()

		pos.X += vel.X * ctx.Dt
		pos.Y += vel.Y * ctx.Dt
	}
}

// Rotation System 2D - Applies angular velocity
func RotationSystem2D(ctx ecs.SystemContext) {
	q := ecs.SimpleQuery2[Rotation2D, AngularVelocity2D](ctx.World)

	for row := range q {
		rot := row.GetMutable1()
		angVel := row.Get2()

		rot.Angle += angVel.Value * ctx.Dt
	}
}

// Rotation System 3D - Applies angular velocity
func RotationSystem(ctx ecs.SystemContext) {
	q := ecs.SimpleQuery2[Rotation, AngularVelocity](ctx.World)

	for row := range q {
		rot := row.GetMutable1()
		angVel := row.Get2()

		rot.X += angVel.X * ctx.Dt
		rot.Y += angVel.Y * ctx.Dt
		rot.Z += angVel.Z * ctx.Dt
	}
}

// Physics System 3D - Applies forces and acceleration
func PhysicsSystem(ctx ecs.SystemContext) {
	gravity := ecs.GetSingleton[PhysicsConfig](ctx.World)

	q := ecs.SimpleQuery3[Velocity, Acceleration, RigidBody](ctx.World)

	for row := range q {
		vel := row.GetMutable1()
		acc := row.Get2()
		body := row.Get3()

		if body.IsKinematic {
			continue
		}

		// Apply acceleration
		if !body.FreezePositionX {
			vel.X += acc.X * ctx.Dt
		}
		if !body.FreezePositionY {
			vel.Y += acc.Y * ctx.Dt
		}
		if !body.FreezePositionZ {
			vel.Z += acc.Z * ctx.Dt
		}

		// Apply gravity
		if body.UseGravity {
			vel.Y += gravity.Gravity * ctx.Dt
		}

		// Apply drag
		drag := 1.0 - (body.Drag * ctx.Dt)
		if drag < 0 {
			drag = 0
		}
		vel.X *= drag
		vel.Y *= drag
		vel.Z *= drag
	}
}

// Physics System 2D - Applies forces and acceleration
func PhysicsSystem2D(ctx ecs.SystemContext) {
	gravity := ecs.GetSingleton[PhysicsConfig2D](ctx.World)

	q := ecs.SimpleQuery3[Velocity2D, Acceleration2D, RigidBody2D](ctx.World)

	for row := range q {
		vel := row.GetMutable1()
		acc := row.Get2()
		body := row.Get3()

		if body.IsKinematic {
			continue
		}

		// Apply acceleration
		if !body.FreezePositionX {
			vel.X += acc.X * ctx.Dt
		}
		if !body.FreezePositionY {
			vel.Y += acc.Y * ctx.Dt
		}

		// Apply gravity
		if body.UseGravity {
			vel.Y += gravity.Gravity * ctx.Dt
		}

		// Apply drag
		drag := 1.0 - (body.Drag * ctx.Dt)
		if drag < 0 {
			drag = 0
		}
		vel.X *= drag
		vel.Y *= drag
	}
}
