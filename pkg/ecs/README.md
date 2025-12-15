# Package ECS

The ecs package provides an Entity Component System (ECS). It is designed to be simple, fast and flexible.

## What is an ECS

The Entity Component System paradigm is used to seperate data from logic and to provide a way to modularize code.
An entities is an identifier and it does not hold any data on it's own. Components are data; they can be attached
to entities. Systems can query, mutate and process those components.

## Todos

- Better Queries
- Better System scheduling
- Query caching
