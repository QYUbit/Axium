package core

type EventType string

type RoomMessage struct {
	Event  string
	Origin string
	Data   []byte
}

type LifecycleHooks interface {
	OnCreate()
	OnJoin(func(Client) bool)
	OnLeave(func(Client))
	OnDestroy()
	onCreate()
	onJoin(Client) bool
	onLeave(Client)
	onDestroy()
}

type MessageHooks interface {
	OnMessage(func(RoomMessage))
	onMessage(RoomMessage)
}

type Room interface {
	LifecycleHooks
	MessageHooks
}

type GameRoom interface {
	Room
	OnUpdate(func(float64))
	onUpdate(float64)
}
