# Axium

Axium is a modular server-side game framework writen in go.

## Explanation

Building online multiplayer games is hard: You have to deal with netcode, client synchronization and cheat prevention. On top of that, you have to make sure everything remains performant. This project aims to provide tools simplifying this process.

## Packages

Imagine Axium as a toolbox; it consists of multiple packages. Those are:

Note: At the moment there are just two. More are planned.

### ECS

A capable, fast and type safe Entity Component System (ECS), enabeling an organized way to store, edit and filter your game state. Read more: [ECS Docs](https://github.com/QYUbit/Axium/blob/main/pkg/ecs/README.md)

### Transport

This package is an abstraction for bi-directional network communication. Adapters for various transports (e.g. quic, websockets) can implement it's definitions. Transport is useful since it serves as a common interface for the transport layer of a game server. It can be used by future higher level packages like ECS for example.

## Disclaimer

This project is still in development, backwards compatibility and production quality are not guaranteed.

## Contributions

Issues and PRs are welcome.

## License

Axium is governed under the [MIT License](https://github.com/QYUbit/Axium/blob/main/LICENSE)
