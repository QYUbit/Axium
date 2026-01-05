package s2

import (
	"context"
	"fmt"
	"sync"
)

type RoomFactory func(r *Room)

type roomManager struct {
	rooms       map[string]*Room
	roomsMu     sync.RWMutex
	factories   map[string]RoomFactory
	factoriesMu sync.RWMutex
	idGenerator func() string
}

func newRoomManager(idGenerator func() string) *roomManager {
	return &roomManager{
		rooms:       make(map[string]*Room),
		factories:   make(map[string]RoomFactory),
		idGenerator: idGenerator,
	}
}

func (m *roomManager) getRoom(id string) (*Room, bool) {
	m.roomsMu.RLock()
	r, ok := m.rooms[id]
	m.roomsMu.RUnlock()
	return r, ok
}

func (m *roomManager) registerFactory(typ string, f RoomFactory) {
	m.factoriesMu.Lock()
	m.factories[typ] = f
	m.factoriesMu.Unlock()
}

func (m *roomManager) createRoom(ctx context.Context, typ string) (*Room, error) {
	m.factoriesMu.RLock()
	f, ok := m.factories[typ]
	m.factoriesMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("room factory for type %s not found", typ)
	}

	ctx, cancel := context.WithCancel(ctx)

	id := m.idGenerator()
	r := newRoom(cancel, id)

	m.roomsMu.Lock()
	m.rooms[id] = r
	m.roomsMu.Unlock()

	f(r)
	r.onCreate(ctx)
	return r, nil
}
