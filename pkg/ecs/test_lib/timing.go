package lib

import "github.com/QYUbit/Axium/pkg/ecs"

type Lifetime struct {
	MaxAge          float64
	CurrentAge      float64
	DestroyOnExpire bool
}

func (Lifetime) Id() ecs.ComponentID { return 30 }

type Cooldown struct {
	Cooldowns map[string]CooldownTimer
}

type CooldownTimer struct {
	Duration  float64
	Remaining float64
	Ready     bool
}

func (Cooldown) Id() ecs.ComponentID { return 31 }

type Timer struct {
	Duration   float64
	Elapsed    float64
	Repeat     bool
	Active     bool
	OnComplete string // Event name to trigger
}

func (Timer) Id() ecs.ComponentID { return 32 }

func LifetimeSystem(ctx ecs.SystemContext) {
	q := ecs.SimpleQuery1[Lifetime](ctx.World)

	for row := range q {
		lifetime := row.GetMutable()
		lifetime.CurrentAge += ctx.Dt

		if lifetime.CurrentAge >= lifetime.MaxAge && lifetime.DestroyOnExpire {
			ctx.Commands.DestroyEntity(row.ID)
		}
	}
}

// Cooldown System - Updates all cooldowns
func CooldownSystem(ctx ecs.SystemContext) {
	q := ecs.SimpleQuery1[Cooldown](ctx.World)

	for row := range q {
		cooldowns := row.GetMutable()

		for key, timer := range cooldowns.Cooldowns {
			if !timer.Ready {
				timer.Remaining -= ctx.Dt
				if timer.Remaining <= 0 {
					timer.Remaining = 0
					timer.Ready = true
				}
				cooldowns.Cooldowns[key] = timer
			}
		}
	}
}

// Timer System - Updates generic timers
func TimerSystem(ctx ecs.SystemContext) {
	q := ecs.SimpleQuery1[Timer](ctx.World)

	for row := range q {
		timer := row.GetMutable()

		if !timer.Active {
			continue
		}

		timer.Elapsed += ctx.Dt

		if timer.Elapsed >= timer.Duration {
			// TODO: Trigger event system
			if timer.Repeat {
				timer.Elapsed = 0
			} else {
				timer.Active = false
			}
		}
	}
}
