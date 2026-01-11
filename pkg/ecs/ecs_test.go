package ecs

import (
	"testing"
	"time"
)

// Test Components
type TestPosition struct{ X, Y float64 }
type TestVelocity struct{ X, Y float64 }
type TestHealth struct{ Value int }
type TestName struct{ Name string }
type TestTag struct{}

// TestEngineCreation tests basic engine creation
func TestEngineCreation(t *testing.T) {
	engine := NewEngine()
	if engine == nil {
		t.Fatal("Engine creation failed")
	}
	if engine.World() == nil {
		t.Error("Engine world is nil")
	}
	if engine.Scheduler() == nil {
		t.Error("Engine scheduler is nil")
	}
}

// TestComponentRegistration tests component registration
func TestComponentRegistration(t *testing.T) {
	engine := NewEngine()

	RegisterComponent[TestPosition](engine, 0)
	RegisterComponent[TestVelocity](engine, 1)
	RegisterComponentAuto[TestHealth](engine)

	if len(engine.World().stores) != 3 {
		t.Errorf("Expected 3 stores, got %d", len(engine.World().stores))
	}
}

// TestComponentRegistrationDuplicate tests duplicate registration handling
func TestComponentRegistrationDuplicate(t *testing.T) {
	engine := NewEngine()

	RegisterComponent[TestPosition](engine, 0)
	RegisterComponent[TestPosition](engine, 0)

	if len(engine.World().stores) != 1 {
		t.Errorf("Expected 1 store after duplicate registration, got %d", len(engine.World().stores))
	}
}

// TestComponentRegistrationPanic tests that duplicate IDs panic
func TestComponentRegistrationPanic(t *testing.T) {
	engine := NewEngine()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for duplicate component ID")
		}
	}()

	RegisterComponent[TestPosition](engine, 0)
	RegisterComponent[TestVelocity](engine, 0) // Should panic
}

// TestEntityCreation tests entity creation via commands
func TestEntityCreation(t *testing.T) {
	engine := NewEngine()
	RegisterComponent[TestPosition](engine, 0)

	called := false
	engine.RegisterSystemFunc(func(ctx SystemContext) {
		ctx.Commands.CreateEntity(Entity(1), TestPosition{X: 10, Y: 20})
		called = true
	}, Trigger(OnStartup))

	engine.Scheduler().Compile()
	engine.Scheduler().RunInit(engine.World())

	if !called {
		t.Error("System was not called")
	}

	if !engine.World().entityExists(Entity(1)) {
		t.Error("Entity was not created")
	}
}

// TestEntityDestruction tests entity destruction
func TestEntityDestruction(t *testing.T) {
	engine := NewEngine()
	RegisterComponent[TestPosition](engine, 0)

	var setupCalled, destroyCalled bool

	engine.RegisterSystemFunc(func(ctx SystemContext) {
		ctx.Commands.CreateEntity(Entity(1), TestPosition{X: 10, Y: 20})
		setupCalled = true
	}, Trigger(OnStartup))

	engine.RegisterSystemFunc(func(ctx SystemContext) {
		if setupCalled && !destroyCalled {
			ctx.Commands.DestroyEntity(Entity(1))
			destroyCalled = true
		}
	})

	engine.Scheduler().Compile()
	engine.Scheduler().RunInit(engine.World())

	if !setupCalled {
		t.Error("Setup system was not called")
	}

	if !engine.World().entityExists(Entity(1)) {
		t.Error("Entity was not created in setup")
	}

	engine.ExecuteTick(1.0 / 60.0)

	if engine.World().entityExists(Entity(1)) {
		t.Error("Entity was not destroyed")
	}
}

// TestComponentAddRemove tests adding and removing components
func TestComponentAddRemove(t *testing.T) {
	engine := NewEngine()
	RegisterComponent[TestPosition](engine, 0)
	RegisterComponent[TestVelocity](engine, 1)

	engine.RegisterSystemFunc(func(ctx SystemContext) {
		ctx.Commands.CreateEntity(Entity(1), TestPosition{X: 10, Y: 20})
	}, Trigger(OnStartup))

	tickCount := 0
	engine.RegisterSystemFunc(func(ctx SystemContext) {
		tickCount++
		switch tickCount {
		case 1:
			ctx.Commands.AddComponent(Entity(1), TestVelocity{X: 1, Y: 1})
		case 2:
			if pos, ok := Get[TestPosition](ctx.World, Entity(1)); !ok {
				t.Error("Position component missing")
			} else if pos.X != 10 {
				t.Error("Position component corrupted")
			}
			if _, ok := Get[TestVelocity](ctx.World, Entity(1)); !ok {
				t.Error("Velocity component not added")
			}
			ctx.Commands.RemoveComponent(Entity(1), TestVelocity{})
		}
	})

	engine.Scheduler().Compile()
	engine.Scheduler().RunInit(engine.World())
	engine.ExecuteTick(1.0 / 60.0)
	engine.ExecuteTick(1.0 / 60.0)
}

// TestQuery1 tests single component queries
func TestQuery1(t *testing.T) {
	engine := NewEngine()
	RegisterComponent[TestPosition](engine, 0)

	engine.RegisterSystemFunc(func(ctx SystemContext) {
		ctx.Commands.CreateEntity(Entity(1), TestPosition{X: 10, Y: 20})
		ctx.Commands.CreateEntity(Entity(2), TestPosition{X: 30, Y: 40})
	}, Trigger(OnStartup))

	count := 0
	engine.RegisterSystemFunc(func(ctx SystemContext) {
		q := Query1[TestPosition](ctx.World)
		for row := range q.Iter() {
			count++
			pos := row.Get()
			if pos.X != 10 && pos.X != 30 {
				t.Errorf("Unexpected position value: %f", pos.X)
			}
		}
	})

	engine.Scheduler().Compile()
	engine.Scheduler().RunInit(engine.World())
	engine.ExecuteTick(1.0 / 60.0)

	if count != 2 {
		t.Errorf("Expected to find 2 entities, found %d", count)
	}
}

// TestQuery2 tests two component queries
func TestQuery2(t *testing.T) {
	engine := NewEngine()
	RegisterComponent[TestPosition](engine, 0)
	RegisterComponent[TestVelocity](engine, 1)

	engine.RegisterSystemFunc(func(ctx SystemContext) {
		ctx.Commands.CreateEntity(Entity(1), TestPosition{X: 10, Y: 20}, TestVelocity{X: 1, Y: 1})
		ctx.Commands.CreateEntity(Entity(2), TestPosition{X: 30, Y: 40})
	}, Trigger(OnStartup))

	count := 0
	engine.RegisterSystemFunc(func(ctx SystemContext) {
		q := Query2[TestPosition, TestVelocity](ctx.World)
		for row := range q.Iter() {
			count++
			pos := row.Get1()
			vel := row.Get2()
			if pos == nil || vel == nil {
				t.Error("Component is nil")
			}
		}
	})

	engine.Scheduler().Compile()
	engine.Scheduler().RunInit(engine.World())
	engine.ExecuteTick(1.0 / 60.0)

	if count != 1 {
		t.Errorf("Expected 1 entity with both components, got %d", count)
	}
}

// TestQueryWithRequire tests queries with additional requirements
func TestQueryWithRequire(t *testing.T) {
	engine := NewEngine()
	RegisterComponent[TestPosition](engine, 0)
	RegisterComponent[TestVelocity](engine, 1)
	RegisterComponent[TestHealth](engine, 2)

	engine.RegisterSystemFunc(func(ctx SystemContext) {
		ctx.Commands.CreateEntity(Entity(1), TestPosition{}, TestVelocity{}, TestHealth{Value: 100})
		ctx.Commands.CreateEntity(Entity(2), TestPosition{}, TestVelocity{})
	}, Trigger(OnStartup))

	count := 0
	engine.RegisterSystemFunc(func(ctx SystemContext) {
		q := Query2[TestPosition, TestVelocity](ctx.World, Require(TestHealth{}))
		for range q.Iter() {
			count++
		}
	})

	engine.Scheduler().Compile()
	engine.Scheduler().RunInit(engine.World())
	engine.ExecuteTick(1.0 / 60.0)

	if count != 1 {
		t.Errorf("Expected 1 entity with Health, got %d", count)
	}
}

// TestQueryWithExclude tests queries with exclusions
func TestQueryWithExclude(t *testing.T) {
	engine := NewEngine()
	RegisterComponent[TestPosition](engine, 0)
	RegisterComponent[TestHealth](engine, 1)

	engine.RegisterSystemFunc(func(ctx SystemContext) {
		ctx.Commands.CreateEntity(Entity(1), TestPosition{}, TestHealth{})
		ctx.Commands.CreateEntity(Entity(2), TestPosition{})
	}, Trigger(OnStartup))

	count := 0
	engine.RegisterSystemFunc(func(ctx SystemContext) {
		q := Query1[TestPosition](ctx.World, Exclude(TestHealth{}))
		for range q.Iter() {
			count++
		}
	})

	engine.Scheduler().Compile()
	engine.Scheduler().RunInit(engine.World())
	engine.ExecuteTick(1.0 / 60.0)

	if count != 1 {
		t.Errorf("Expected 1 entity without Health, got %d", count)
	}
}

// TestMutation tests component mutation
func TestMutation(t *testing.T) {
	engine := NewEngine()
	RegisterComponent[TestPosition](engine, 0)

	engine.RegisterSystemFunc(func(ctx SystemContext) {
		ctx.Commands.CreateEntity(Entity(1), TestPosition{X: 0, Y: 0})
	}, Trigger(OnStartup))

	engine.RegisterSystemFunc(func(ctx SystemContext) {
		q := Query1[TestPosition](ctx.World)
		for row := range q.Iter() {
			pos := row.Mut()
			pos.X += 10
			pos.Y += 20
		}
	}, Writes(TestPosition{}))

	engine.Scheduler().Compile()
	engine.Scheduler().RunInit(engine.World())
	engine.ExecuteTick(1.0 / 60.0)

	pos, ok := Get[TestPosition](engine.World(), Entity(1))
	if !ok {
		t.Fatal("Position component not found")
	}
	if pos.X != 10 || pos.Y != 20 {
		t.Errorf("Position not mutated correctly: got (%f, %f), want (10, 20)", pos.X, pos.Y)
	}
}

// TestSingleton tests singleton functionality
func TestSingleton(t *testing.T) {
	engine := NewEngine()

	type Counter struct{ Value int }
	RegisterSingleton(engine, Counter{Value: 0})

	engine.RegisterSystemFunc(func(ctx SystemContext) {
		counter, ok := GetSingleton[Counter](ctx.World)
		if !ok {
			t.Error("Singleton not found")
			return
		}
		counter.Mut().Value++
	}, Writes(Counter{}))

	engine.Scheduler().Compile()

	for range 5 {
		engine.ExecuteTick(1.0 / 60.0)
	}

	counter, ok := GetSingleton[Counter](engine.World())
	if !ok {
		t.Fatal("Singleton not found")
	}
	if counter.Get().Value != 5 {
		t.Errorf("Expected counter value 5, got %d", counter.Get().Value)
	}
}

// TestMessages tests message passing
func TestMessages(t *testing.T) {
	engine := NewEngine()

	type TestMessage struct{ Value int }
	RegisterMessage[TestMessage](engine)

	receivedCount := 0
	receivedValues := []int{}

	engine.RegisterSystemFunc(func(ctx SystemContext) {
		PushMessage(ctx.World, TestMessage{Value: 42})
	})

	engine.RegisterSystemFunc(func(ctx SystemContext) {
		messages := CollectMessages[TestMessage](ctx.World)
		receivedCount += len(messages)
		for _, msg := range messages {
			receivedValues = append(receivedValues, msg.Value)
		}
	})

	engine.Scheduler().Compile()

	for range 3 {
		engine.ExecuteTick(1.0 / 60.0)
	}

	if receivedCount != 2 {
		t.Errorf("Expected 2 messages, received %d", receivedCount)
	}

	for _, val := range receivedValues {
		if val != 42 {
			t.Errorf("Expected message value 42, got %d", val)
		}
	}
}

// TestOnStartupTrigger tests that OnStartup systems run once
func TestOnStartupTrigger(t *testing.T) {
	engine := NewEngine()

	count := 0
	engine.RegisterSystemFunc(func(ctx SystemContext) {
		count++
	}, Trigger(OnStartup))

	engine.Scheduler().Compile()
	engine.Scheduler().RunInit(engine.World())

	for range 5 {
		engine.ExecuteTick(1.0 / 60.0)
	}

	if count != 1 {
		t.Errorf("OnStartup system should run once, ran %d times", count)
	}
}

// TestDeltaTime tests that systems receive delta time
func TestDeltaTime(t *testing.T) {
	engine := NewEngine()

	receivedDt := 0.0
	engine.RegisterSystemFunc(func(ctx SystemContext) {
		if ctx.Dt > 0 {
			receivedDt = ctx.Dt
		}
	})

	engine.Scheduler().Compile()

	expectedDt := 1.0 / 60.0
	engine.ExecuteTick(expectedDt)

	if receivedDt == 0 {
		t.Error("System did not receive delta time")
	}

	if receivedDt != expectedDt {
		t.Errorf("Expected dt %f, got %f", expectedDt, receivedDt)
	}
}

// TestRegistrationAfterRun tests that registration after Run panics
func TestRegistrationAfterRun(t *testing.T) {
	ctx := t.Context()
	engine := NewEngine()

	go engine.Run(ctx, 60)
	time.Sleep(50 * time.Millisecond)

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when registering after Run")
		}
	}()

	RegisterComponent[TestPosition](engine, 0)

	engine.Close()
}

// TestEmptyQuery tests querying for non-existent components
func TestEmptyQuery(t *testing.T) {
	engine := NewEngine()

	count := 0
	engine.RegisterSystemFunc(func(ctx SystemContext) {
		q := Query1[TestPosition](ctx.World)
		for range q.Iter() {
			count++
		}
	})

	engine.Scheduler().Compile()
	engine.ExecuteTick(1.0 / 60.0)

	if count != 0 {
		t.Errorf("Empty query should return 0 results, got %d", count)
	}
}

// TestMultipleEntitiesIteration tests iteration over multiple entities
func TestMultipleEntitiesIteration(t *testing.T) {
	engine := NewEngine()
	RegisterComponent[TestPosition](engine, 0)

	engine.RegisterSystemFunc(func(ctx SystemContext) {
		for i := range 100 {
			ctx.Commands.CreateEntity(Entity(i), TestPosition{X: float64(i), Y: float64(i * 2)})
		}
	}, Trigger(OnStartup))

	count := 0
	sumX := 0.0

	engine.RegisterSystemFunc(func(ctx SystemContext) {
		q := Query1[TestPosition](ctx.World)
		for row := range q.Iter() {
			count++
			sumX += row.Get().X
		}
	})

	engine.Scheduler().Compile()
	engine.Scheduler().RunInit(engine.World())
	engine.ExecuteTick(1.0 / 60.0)

	if count != 100 {
		t.Errorf("Expected 100 entities, got %d", count)
	}

	expectedSum := 0.0
	for i := range 100 {
		expectedSum += float64(i)
	}

	if sumX != expectedSum {
		t.Errorf("Sum mismatch: got %f, want %f", sumX, expectedSum)
	}
}

// TestEntitySpecificQuery tests queries for specific entities
func TestEntitySpecificQuery(t *testing.T) {
	engine := NewEngine()
	RegisterComponent[TestPosition](engine, 0)

	engine.RegisterSystemFunc(func(ctx SystemContext) {
		for i := range 10 {
			ctx.Commands.CreateEntity(Entity(i), TestPosition{X: float64(i), Y: float64(i)})
		}
	}, Trigger(OnStartup))

	count := 0
	engine.RegisterSystemFunc(func(ctx SystemContext) {
		q := Query1[TestPosition](ctx.World, Entities(Entity(3), Entity(5), Entity(7)))
		for range q.Iter() {
			count++
		}
	})

	engine.Scheduler().Compile()
	engine.Scheduler().RunInit(engine.World())
	engine.ExecuteTick(1.0 / 60.0)

	if count != 3 {
		t.Errorf("Expected 3 specific entities, got %d", count)
	}
}

// TestExcludeEntitiesQuery tests queries excluding specific entities
func TestExcludeEntitiesQuery(t *testing.T) {
	engine := NewEngine()
	RegisterComponent[TestPosition](engine, 0)

	engine.RegisterSystemFunc(func(ctx SystemContext) {
		for i := range 10 {
			ctx.Commands.CreateEntity(Entity(i), TestPosition{X: float64(i), Y: float64(i)})
		}
	}, Trigger(OnStartup))

	count := 0
	engine.RegisterSystemFunc(func(ctx SystemContext) {
		q := Query1[TestPosition](ctx.World, ExcludeEntities(Entity(3), Entity(5), Entity(7)))
		for range q.Iter() {
			count++
		}
	})

	engine.Scheduler().Compile()
	engine.Scheduler().RunInit(engine.World())
	engine.ExecuteTick(1.0 / 60.0)

	if count != 7 {
		t.Errorf("Expected 7 entities (10 - 3 excluded), got %d", count)
	}
}
