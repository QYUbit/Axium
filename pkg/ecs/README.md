# Package ECS

The ecs package provides an Entity Component System (ECS). It is designed to be capable, fast and flexible.

## What is an ECS

The Entity Component System paradigm is used to seperate data from logic and to provide a way to modularize code.
An entity is just an identifier and it does not hold any data on it's own. Components are pure data; they can be
attached to entities. Systems can query, mutate and process those components.

## Usage

A component can be any value. But it is recomended to only use structs as components.

```Go
type Position struct {
    X float64
    Y float64
}
```

The ECSEngine is the main orchestrator of the ecs package. It can be created with `NewEngine()`.

Components must first get registered to be able to be used. Use `RegisterComponent[T](engine *ECSEngine, id uint16)` or `AutoRegisterComponent[T](engine *ECSEngine)`

```Go
type Position struct {
    X float64
    Y float64
} 

engine := NewEngine()

AutoRegisterComponent[Position](engine) // Generates a component id internally
```

To interact with the worlds data you have to use systems. A system is a funtion that takes a `SystemContext` as an argument. Systems can be registered with `ecs.RegisterSystem(system func(SystemContext))`.

```Go
func MySystem(ctx SystemContext) {
    // Do stuff
}

engine := NewEngine()

engine.RegisterSystemFunc(MySystem)
```

To access components, query them from the world.

```Go
type Name struct { Name string }
type Person struct{} // Use empty structs for flags

func MySystem(ctx SystemContext) {
    query := Query1[Name](Require(Person{})) // Will query every entity with components Name and Person. But iterates only over Name components.

    for row := range query {
        name := row.Get()
        entity := row.E
        fmt.Printf("Name of %d: %s\n", entity, name)
    }
}
```

To create / destroy / add components to / remove components from entities, you have to queue those operations in a command buffer. The mentioned operations are special and will only be executed at the end of the current tick.

```Go
func MySystem(ctx SystemContext) {
    ctx.Commands.CreateEntity(Entity(0), Position{0, 0}, Velocity{}) // Entity value and optional initial components

    ctx.Commands.AddComponent(Entity(0), Name{"John Doe"})
}
```

The ecs package also provides singletons. They are just data, but compared to components, they can not be attached to any entities.

```Go
type TickCounter struct {
    Tick int
}

engine := NewEngine()

RegisterSingleton[TickCounter](engine)
```

```Go
type TickCounter struct {
    Tick int
}

func IncreseTickSystem(ctx SystemContext) {
    counter := GetSingleton[TickCounter](ctx.World)
    counter.Mut().Tick++
}
```

Messages are used to communicate between systems and can even be used for communication outside of systems.

Note: For accesses to message queues outside of systems you have to use `PushMessageSafe[T](msg T)` and `CollectMessagesSafe[T]() []T` respectivly.

```Go
type SomeMessage struct {
    Payload string
}

func MySystem(ctx SystemContext) {
    messages := CollectMessages(ctx.World)
    for _, msg := range messages {
        fmt.Println(msg.Payload)
    }
}

engine := NewEngine()

RegisterMessage[SomeMessage](engine)

engine.RegisterSystem(MySystem)

PushMessageSafe(engine, SomeMessage{Payload: "Hello!"})
```

Most* methods to interact with the world are not thread-safe, since locks are avoided to increase performance. To circumvent data races, you have to specify the respective dependencies for each system.

*Note: Commands and message-reads are thread-safe, other world accesses are not.

```Go
type Position struct {X, Y float64}
type Velocity struct {X, Y float64}

RegisterSystemFunc(
    engine,
    Reads(Velocity{}),
    Writes(Position{}),
)
```

## Event Loop

`(*ecs.ECSEngine).Run()` starts the event loop. `(*ecs.ECSEngine).Close()` waits until the current tick is completed and stops the loop. All registrations must be completed before `(*ecs.ECSEngine).Run()` is called.

Tick flow:

1. All update systems are being executed.

2. Command buffers get merged into one.

3. All end-of-tick systems are being executed (with the merged command buffer).

4. Command buffer are executed.

5. Message queues are swapped.

## Todos

- Debug tools & Testing
