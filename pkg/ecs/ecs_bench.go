package ecs

import (
	"testing"
)

// TODO Move benchmarks in seperate package, check validity of benchmarks

// Benchmark Components
type BenchPosition struct{ X, Y, Z float64 }
type BenchVelocity struct{ X, Y, Z float64 }
type BenchHealth struct{ Current, Max int }
type BenchDamage struct{ Value int }
type BenchArmor struct{ Value int }

// BenchmarkEntityCreation benchmarks entity creation
func BenchmarkEntityCreation(b *testing.B) {
	engine := NewEngine()
	RegisterComponent[BenchPosition](engine, 0)

	b.ResetTimer()
	for b.Loop() {
		cb := &CommandBuffer{}
		cb.CreateEntity(Entity(b.N), BenchPosition{X: 1, Y: 2, Z: 3})
		engine.world.processCommands(cb.GetCommands())
	}
}

// BenchmarkEntityCreationWithMultipleComponents benchmarks entity creation with multiple components
func BenchmarkEntityCreationWithMultipleComponents(b *testing.B) {
	engine := NewEngine()
	RegisterComponent[BenchPosition](engine, 0)
	RegisterComponent[BenchVelocity](engine, 1)
	RegisterComponent[BenchHealth](engine, 2)

	b.ResetTimer()
	for b.Loop() {
		cb := &CommandBuffer{}
		cb.CreateEntity(
			Entity(b.N),
			BenchPosition{X: 1, Y: 2, Z: 3},
			BenchVelocity{X: 0.1, Y: 0.2, Z: 0.3},
			BenchHealth{Current: 100, Max: 100},
		)
		engine.world.processCommands(cb.GetCommands())
	}
}

// BenchmarkEntityDestruction benchmarks entity destruction
func BenchmarkEntityDestruction(b *testing.B) {
	engine := NewEngine()
	RegisterComponent[BenchPosition](engine, 0)

	// Create entities first
	cb := &CommandBuffer{}
	for i := range b.N {
		cb.CreateEntity(Entity(i), BenchPosition{X: 1, Y: 2, Z: 3})
	}
	engine.world.processCommands(cb.GetCommands())

	b.ResetTimer()
	for b.Loop() {
		cb := &CommandBuffer{}
		cb.DestroyEntity(Entity(b.N))
		engine.world.processCommands(cb.GetCommands())
	}
}

// BenchmarkComponentAdd benchmarks adding components to existing entities
func BenchmarkComponentAdd(b *testing.B) {
	engine := NewEngine()
	RegisterComponent[BenchPosition](engine, 0)
	RegisterComponent[BenchVelocity](engine, 1)

	// Create entities
	cb := &CommandBuffer{}
	for i := range b.N {
		cb.CreateEntity(Entity(i), BenchPosition{X: 1, Y: 2, Z: 3})
	}
	engine.world.processCommands(cb.GetCommands())

	b.ResetTimer()
	for b.Loop() {
		cb := &CommandBuffer{}
		cb.AddComponent(Entity(b.N), BenchVelocity{X: 0.1, Y: 0.2, Z: 0.3})
		engine.world.processCommands(cb.GetCommands())
	}
}

// BenchmarkComponentRemove benchmarks removing components
func BenchmarkComponentRemove(b *testing.B) {
	engine := NewEngine()
	RegisterComponent[BenchPosition](engine, 0)
	RegisterComponent[BenchVelocity](engine, 1)

	// Create entities with both components
	cb := &CommandBuffer{}
	for i := range b.N {
		cb.CreateEntity(Entity(i), BenchPosition{}, BenchVelocity{})
	}
	engine.world.processCommands(cb.GetCommands())

	b.ResetTimer()
	for b.Loop() {
		cb := &CommandBuffer{}
		cb.RemoveComponent(Entity(b.N), BenchVelocity{})
		engine.world.processCommands(cb.GetCommands())
	}
}

// BenchmarkQuery1_10 benchmarks querying 10 entities
func BenchmarkQuery1_10(b *testing.B) {
	benchmarkQuery1(b, 10)
}

// BenchmarkQuery1_100 benchmarks querying 100 entities
func BenchmarkQuery1_100(b *testing.B) {
	benchmarkQuery1(b, 100)
}

// BenchmarkQuery1_1000 benchmarks querying 1000 entities
func BenchmarkQuery1_1000(b *testing.B) {
	benchmarkQuery1(b, 1000)
}

// BenchmarkQuery1_10000 benchmarks querying 10000 entities
func BenchmarkQuery1_10000(b *testing.B) {
	benchmarkQuery1(b, 10000)
}

func benchmarkQuery1(b *testing.B, entityCount int) {
	engine := NewEngine()
	RegisterComponent[BenchPosition](engine, 0)

	// Create entities
	cb := &CommandBuffer{}
	for i := range entityCount {
		cb.CreateEntity(Entity(i), BenchPosition{X: float64(i), Y: float64(i * 2), Z: float64(i * 3)})
	}
	engine.world.processCommands(cb.GetCommands())

	b.ResetTimer()
	for b.Loop() {
		q := Query1[BenchPosition](engine.world)
		for range q.Iter() {
			// Just iterate
		}
	}
}

// BenchmarkQuery2_10 benchmarks querying 10 entities with 2 components
func BenchmarkQuery2_10(b *testing.B) {
	benchmarkQuery2(b, 10)
}

// BenchmarkQuery2_100 benchmarks querying 100 entities with 2 components
func BenchmarkQuery2_100(b *testing.B) {
	benchmarkQuery2(b, 100)
}

// BenchmarkQuery2_1000 benchmarks querying 1000 entities with 2 components
func BenchmarkQuery2_1000(b *testing.B) {
	benchmarkQuery2(b, 1000)
}

// BenchmarkQuery2_10000 benchmarks querying 10000 entities with 2 components
func BenchmarkQuery2_10000(b *testing.B) {
	benchmarkQuery2(b, 10000)
}

func benchmarkQuery2(b *testing.B, entityCount int) {
	engine := NewEngine()
	RegisterComponent[BenchPosition](engine, 0)
	RegisterComponent[BenchVelocity](engine, 1)

	// Create entities with both components
	cb := &CommandBuffer{}
	for i := range entityCount {
		cb.CreateEntity(
			Entity(i),
			BenchPosition{X: float64(i), Y: float64(i * 2), Z: float64(i * 3)},
			BenchVelocity{X: 0.1, Y: 0.2, Z: 0.3},
		)
	}
	engine.world.processCommands(cb.GetCommands())

	b.ResetTimer()
	for b.Loop() {
		q := Query2[BenchPosition, BenchVelocity](engine.world)
		for range q.Iter() {
			// Just iterate
		}
	}
}

// BenchmarkQuery3_1000 benchmarks querying 1000 entities with 3 components
func BenchmarkQuery3_1000(b *testing.B) {
	engine := NewEngine()
	RegisterComponent[BenchPosition](engine, 0)
	RegisterComponent[BenchVelocity](engine, 1)
	RegisterComponent[BenchHealth](engine, 2)

	// Create entities with three components
	cb := &CommandBuffer{}
	for i := range 1000 {
		cb.CreateEntity(
			Entity(i),
			BenchPosition{X: float64(i), Y: float64(i * 2), Z: float64(i * 3)},
			BenchVelocity{X: 0.1, Y: 0.2, Z: 0.3},
			BenchHealth{Current: 100, Max: 100},
		)
	}
	engine.world.processCommands(cb.GetCommands())

	b.ResetTimer()
	for b.Loop() {
		q := Query3[BenchPosition, BenchVelocity, BenchHealth](engine.world)
		for range q.Iter() {
			// Just iterate
		}
	}
}

// BenchmarkQueryWithRequire benchmarks queries with additional requirements
func BenchmarkQueryWithRequire(b *testing.B) {
	engine := NewEngine()
	RegisterComponent[BenchPosition](engine, 0)
	RegisterComponent[BenchVelocity](engine, 1)
	RegisterComponent[BenchHealth](engine, 2)

	// Create entities, half with health
	cb := &CommandBuffer{}
	for i := range 1000 {
		if i%2 == 0 {
			cb.CreateEntity(
				Entity(i),
				BenchPosition{},
				BenchVelocity{},
				BenchHealth{Current: 100, Max: 100},
			)
		} else {
			cb.CreateEntity(Entity(i), BenchPosition{}, BenchVelocity{})
		}
	}
	engine.world.processCommands(cb.GetCommands())

	b.ResetTimer()
	for b.Loop() {
		q := Query2[BenchPosition, BenchVelocity](engine.world, Require(BenchHealth{}))
		for range q.Iter() {
			// Just iterate
		}
	}
}

// BenchmarkQueryWithExclude benchmarks queries with exclusions
func BenchmarkQueryWithExclude(b *testing.B) {
	engine := NewEngine()
	RegisterComponent[BenchPosition](engine, 0)
	RegisterComponent[BenchVelocity](engine, 1)
	RegisterComponent[BenchHealth](engine, 2)

	// Create entities, half with health
	cb := &CommandBuffer{}
	for i := range 1000 {
		if i%2 == 0 {
			cb.CreateEntity(
				Entity(i),
				BenchPosition{},
				BenchVelocity{},
				BenchHealth{Current: 100, Max: 100},
			)
		} else {
			cb.CreateEntity(Entity(i), BenchPosition{}, BenchVelocity{})
		}
	}
	engine.world.processCommands(cb.GetCommands())

	b.ResetTimer()
	for b.Loop() {
		q := Query2[BenchPosition, BenchVelocity](engine.world, Exclude(BenchHealth{}))
		for range q.Iter() {
			// Just iterate
		}
	}
}

// BenchmarkComponentGet benchmarks getting component data
func BenchmarkComponentGet(b *testing.B) {
	engine := NewEngine()
	RegisterComponent[BenchPosition](engine, 0)

	// Create entity
	cb := &CommandBuffer{}
	cb.CreateEntity(Entity(1), BenchPosition{X: 10, Y: 20, Z: 30})
	engine.world.processCommands(cb.GetCommands())

	b.ResetTimer()
	for b.Loop() {
		pos, _ := Get[BenchPosition](engine.world, Entity(1))
		_ = pos.X
	}
}

// BenchmarkComponentMut benchmarks mutating component data
func BenchmarkComponentMut(b *testing.B) {
	engine := NewEngine()
	RegisterComponent[BenchPosition](engine, 0)

	// Create entity
	cb := &CommandBuffer{}
	cb.CreateEntity(Entity(1), BenchPosition{X: 10, Y: 20, Z: 30})
	engine.world.processCommands(cb.GetCommands())

	b.ResetTimer()
	for b.Loop() {
		pos, _ := GetMut[BenchPosition](engine.world, Entity(1))
		pos.X += 1
	}
}

// BenchmarkIterateAndMutate benchmarks iterating and mutating components
func BenchmarkIterateAndMutate(b *testing.B) {
	engine := NewEngine()
	RegisterComponent[BenchPosition](engine, 0)
	RegisterComponent[BenchVelocity](engine, 1)

	// Create entities
	cb := &CommandBuffer{}
	for i := range 1000 {
		cb.CreateEntity(
			Entity(i),
			BenchPosition{X: float64(i), Y: float64(i * 2), Z: float64(i * 3)},
			BenchVelocity{X: 0.1, Y: 0.2, Z: 0.3},
		)
	}
	engine.world.processCommands(cb.GetCommands())

	b.ResetTimer()
	for b.Loop() {
		q := Query2[BenchPosition, BenchVelocity](engine.world)
		for row := range q.Iter() {
			pos := row.Mut1()
			vel := row.Get2()
			pos.X += vel.X
			pos.Y += vel.Y
			pos.Z += vel.Z
		}
	}
}

// BenchmarkSingletonGet benchmarks getting singleton data
func BenchmarkSingletonGet(b *testing.B) {
	engine := NewEngine()

	type GlobalConfig struct{ Speed float64 }
	RegisterSingleton(engine, GlobalConfig{Speed: 1.0})

	b.ResetTimer()
	for b.Loop() {
		config, _ := GetSingleton[GlobalConfig](engine.world)
		_ = config.Get().Speed
	}
}

// BenchmarkSingletonMut benchmarks mutating singleton data
func BenchmarkSingletonMut(b *testing.B) {
	engine := NewEngine()

	type GlobalConfig struct{ Speed float64 }
	RegisterSingleton(engine, GlobalConfig{Speed: 1.0})

	b.ResetTimer()
	for b.Loop() {
		config, _ := GetSingleton[GlobalConfig](engine.world)
		config.Mut().Speed += 0.1
	}
}

// BenchmarkMessagePush benchmarks pushing messages
func BenchmarkMessagePush(b *testing.B) {
	engine := NewEngine()

	type TestMessage struct{ Value int }
	RegisterMessage[TestMessage](engine)

	b.ResetTimer()
	for b.Loop() {
		PushMessage(engine.world, TestMessage{Value: b.N})
	}
}

// BenchmarkMessagePushSafe benchmarks thread-safe message pushing
func BenchmarkMessagePushSafe(b *testing.B) {
	engine := NewEngine()

	type TestMessage struct{ Value int }
	RegisterMessage[TestMessage](engine)

	b.ResetTimer()
	for b.Loop() {
		PushMessageSafe(engine, TestMessage{Value: b.N})
	}
}

// BenchmarkMessageCollect benchmarks collecting messages
func BenchmarkMessageCollect(b *testing.B) {
	engine := NewEngine()

	type TestMessage struct{ Value int }
	RegisterMessage[TestMessage](engine)

	// Push some messages
	for i := range 100 {
		PushMessage(engine.world, TestMessage{Value: i})
	}

	b.ResetTimer()
	for b.Loop() {
		messages := CollectMessages[TestMessage](engine.world)
		_ = len(messages)
	}
}

// BenchmarkCompleteSystemTick benchmarks a complete system tick
func BenchmarkCompleteSystemTick(b *testing.B) {
	engine := NewEngine()
	RegisterComponent[BenchPosition](engine, 0)
	RegisterComponent[BenchVelocity](engine, 1)

	// Setup system
	engine.RegisterSystemFunc(func(ctx SystemContext) {
		for i := range 1000 {
			ctx.Commands.CreateEntity(
				Entity(i),
				BenchPosition{X: float64(i), Y: float64(i * 2), Z: float64(i * 3)},
				BenchVelocity{X: 0.1, Y: 0.2, Z: 0.3},
			)
		}
	}, Trigger(OnStartup))

	// Movement system
	engine.RegisterSystemFunc(func(ctx SystemContext) {
		q := Query2[BenchPosition, BenchVelocity](ctx.World)
		for row := range q.Iter() {
			pos := row.Mut1()
			vel := row.Get2()
			pos.X += vel.X * ctx.Dt
			pos.Y += vel.Y * ctx.Dt
			pos.Z += vel.Z * ctx.Dt
		}
	}, Reads(BenchVelocity{}), Writes(BenchPosition{}))

	engine.scheduler.Compile()
	engine.scheduler.RunInit(engine.world)

	b.ResetTimer()
	for b.Loop() {
		engine.tick(1.0 / 60.0)
	}
}

// BenchmarkParallelSystemsTick benchmarks parallel system execution
func BenchmarkParallelSystemsTick(b *testing.B) {
	engine := NewEngine()
	RegisterComponent[BenchPosition](engine, 0)
	RegisterComponent[BenchVelocity](engine, 1)
	RegisterComponent[BenchHealth](engine, 2)
	RegisterComponent[BenchDamage](engine, 3)

	// Setup
	engine.RegisterSystemFunc(func(ctx SystemContext) {
		for i := range 1000 {
			ctx.Commands.CreateEntity(
				Entity(i),
				BenchPosition{},
				BenchVelocity{X: 0.1, Y: 0.2, Z: 0.3},
				BenchHealth{Current: 100, Max: 100},
				BenchDamage{Value: 10},
			)
		}
	}, Trigger(OnStartup))

	// System 1: Movement (reads velocity, writes position)
	engine.RegisterSystemFunc(func(ctx SystemContext) {
		q := Query2[BenchPosition, BenchVelocity](ctx.World)
		for row := range q.Iter() {
			pos := row.Mut1()
			vel := row.Get2()
			pos.X += vel.X
			pos.Y += vel.Y
			pos.Z += vel.Z
		}
	}, Reads(BenchVelocity{}), Writes(BenchPosition{}))

	// System 2: Health update (reads damage, writes health) - can run parallel with System 1
	engine.RegisterSystemFunc(func(ctx SystemContext) {
		q := Query2[BenchHealth, BenchDamage](ctx.World)
		for row := range q.Iter() {
			health := row.Mut1()
			damage := row.Get2()
			health.Current -= damage.Value
			if health.Current < 0 {
				health.Current = 0
			}
		}
	}, Reads(BenchDamage{}), Writes(BenchHealth{}))

	engine.scheduler.Compile()
	engine.scheduler.RunInit(engine.world)

	b.ResetTimer()
	for b.Loop() {
		engine.tick(1.0 / 60.0)
	}
}

// BenchmarkCommandBufferOperations benchmarks command buffer operations
func BenchmarkCommandBufferOperations(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		cb := &CommandBuffer{}
		cb.CreateEntity(Entity(b.N), BenchPosition{}, BenchVelocity{})
		cb.AddComponent(Entity(b.N), BenchHealth{Current: 100, Max: 100})
		cb.RemoveComponent(Entity(b.N), BenchVelocity{})
		cb.DestroyEntity(Entity(b.N))
	}
}

// BenchmarkSparseSetLookup benchmarks sparse set entity lookup
func BenchmarkSparseSetLookup(b *testing.B) {
	engine := NewEngine()
	RegisterComponent[BenchPosition](engine, 0)

	// Create entities with gaps
	cb := &CommandBuffer{}
	for i := range 1000 {
		if i%2 == 0 {
			cb.CreateEntity(Entity(i), BenchPosition{X: float64(i), Y: float64(i), Z: float64(i)})
		}
	}
	engine.world.processCommands(cb.GetCommands())

	store, _ := getStoreFromWorld[BenchPosition](engine.world)

	b.ResetTimer()
	for b.Loop() {
		// Lookup existing entity
		_ = store.hasEntity(Entity(500))
	}
}

// BenchmarkQueryFilterOptimization benchmarks query filter optimization
func BenchmarkQueryFilterOptimization(b *testing.B) {
	engine := NewEngine()
	RegisterComponent[BenchPosition](engine, 0)
	RegisterComponent[BenchVelocity](engine, 1)
	RegisterComponent[BenchHealth](engine, 2)
	RegisterComponent[BenchDamage](engine, 3)
	RegisterComponent[BenchArmor](engine, 4)

	// Create varied entities
	cb := &CommandBuffer{}
	for i := range 1000 {
		switch i % 4 {
		case 0:
			cb.CreateEntity(Entity(i), BenchPosition{}, BenchVelocity{}, BenchHealth{})
		case 1:
			cb.CreateEntity(Entity(i), BenchPosition{}, BenchHealth{}, BenchArmor{})
		case 2:
			cb.CreateEntity(Entity(i), BenchPosition{}, BenchVelocity{}, BenchDamage{})
		case 3:
			cb.CreateEntity(Entity(i), BenchPosition{}, BenchHealth{}, BenchDamage{}, BenchArmor{})
		}
	}
	engine.world.processCommands(cb.GetCommands())

	b.ResetTimer()
	for b.Loop() {
		// Query with multiple filters
		q := Query2[BenchPosition, BenchHealth](
			engine.world,
			Require(BenchDamage{}),
			Exclude(BenchVelocity{}),
		)
		count := 0
		for range q.Iter() {
			count++
		}
	}
}
