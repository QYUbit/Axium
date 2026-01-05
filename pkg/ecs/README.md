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

engine.RegisterSystem(MySystem)
```

To access components, query them from the world.

```Go
type Name struct { Name string }
type Person struct{} // Use empty structs for flags

func MySystem(ctx SystemContext) {
    query := Query1[Name](Require(Person{})) // Will query every entity with components Name and Person. But iterates only over Name components.

    for row := range query {
        name := row.C
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
    counter.Tick++
}
```

Messages are used to communicate between systems and can even be used for communication outside of systems.

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

PushMessageSafe(engine, SomeMessage{"Hello!"})
```

## Todos

- Enhanced scheduling options
- Query caching
- Dirty queries
- Code gen
- Delta generation
- Debug tools & Testing
