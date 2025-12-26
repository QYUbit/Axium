# Package ECS

The ecs package provides an Entity Component System (ECS). It is designed to be capable, fast and flexible.

## What is an ECS

The Entity Component System paradigm is used to seperate data from logic and to provide a way to modularize code.
An entity is an identifier and it does not hold any data on it's own. Components are pure data; they can be attached
to entities. Systems can query, mutate and process those components.

## Usage

### Register a System

```Go
import (
    "fmt"
    "time"
    "context"
    
    "github.com/QYUbit/Axium/pkg/ecs"
)

func HelloWorldSystem(_ ecs.SystemContext) {
    fmt.Println("Hello World")
}

func main() {
    engine := ecs.NewEngine()

    engine.RegisterSystem(
        HelloWorldSystem, // Function that satisfies func(ecs.SystemContext)
        ecs.OnStartup,    // Incicates when the system will be executed
        nil,
        nil,
    )

    go func(){
        engine.Run(context.Background())
    }()

    time.Sleep(time.Second)
    engine.Close()
}
```

### Entities & components

```Go
import (
    "fmt"
    "time"
    "context"
    
    "github.com/QYUbit/Axium/pkg/ecs"
)

type MyComponent struct {
    SomeData int
}

var player ecs.EntityID = ecs.EntityID(1)

// Use ctx to queue commands or query components
func SetupSystem(ctx ecs.SystemContext) {
    ctx.commands.NewEntity(player)
}

func main() {
    engine := ecs.NewEngine()

    engine.RegisterSystem(
        SetupSystem,
        ecs.OnStartup,
        nil,
        nil,
    )

    go func(){
        engine.Run(context.Background())
    }()

    time.Sleep(time.Second)
    engine.Close()
}
```

This code will register the system `HelloWorldSystem`, run it when `engine.Run()` gets called and quit after one second.

## Todos

- Enhanced scheduling options
- Query caching
- Archetype storage
- Dirty queries
- Phase panics
