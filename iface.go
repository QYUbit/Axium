package axium

import (
	"context"
	"time"
)

type Room interface {
	RoomHooks
	RoomManagement
	SimulationHooks
	SimulationManagement
	TransportHooks
	TransportManagement
	Id() string
	Context() context.Context
}

type RoomHooks interface {
	OnCreate(func())
	OnJoin(func(session string))
	OnLeave(func(session string))
	OnDestroy(func())
}

type RoomManagement interface {
	Kick(session string) error
	Destroy()
	GetMembers() []string
	GetMemberCount() int
}

type SimulationHooks interface {
	OnTick(func(dt time.Duration))
}

type SimulationManagement interface {
	StartSimulation()
	PauseSimulation()
	ResumeSimulation()
	StopSImulation()
	SetTickRate()
}

type TransportHooks interface {
	OnMessage(func(session string, data []byte))
}

type TransportManagement interface {
	Send(session string, data []byte) error
	Broadcast(data []byte) error
	BroadcastExcept(data []byte, exceptions ...string) error
}
