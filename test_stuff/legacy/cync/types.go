package cync

import (
	"sync"
	"time"
)

type EntityId uint64

type ComponentType string

type Serializer interface {
	Serialize() ([]byte, error)
}

type Component interface {
	Serializer
	Type() string
}

type ClientId string

type StateEngine interface {
	CreateEntity() EntityId
	DestroyEntity(e EntityId) bool
	SetComponent(e EntityId, c Component) bool
	RemoveComponent(e EntityId, ct ComponentType) bool
	GetComponent(e EntityId, ct ComponentType) (Component, bool)
	HasComponent(e EntityId, ct ComponentType) bool
	QueryEntities(componentTypes ...ComponentType) []EntityId

	AddClient(c ClientId)
	RemoveClient(c ClientId)

	GenerateDeltas() (map[ClientId]Serializer, error)

	Update(dt time.Duration)
}

type entity struct {
	comps     map[ComponentType]Component
	mu        sync.RWMutex
	isDeleted bool
}

type ChangeEntry struct {
}

type Engine struct {
	entities map[EntityId]*entity
	entityMu sync.RWMutex
}
