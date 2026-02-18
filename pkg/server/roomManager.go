package server

import "sync"

type roomManager struct {
	rooms map[string]Room
	mu    sync.RWMutex
}

func newRoomManager() *roomManager {
	return &roomManager{
		rooms: make(map[string]Room),
	}
}

func (m *roomManager) set(r Room) {
	m.mu.Lock()
	m.rooms[r.ID()] = r
	m.mu.Unlock()
}

func (m *roomManager) get(id string) (Room, bool) {
	m.mu.RLock()
	r, ok := m.rooms[id]
	m.mu.RUnlock()
	return r, ok
}

func (m *roomManager) delete(id string) {
	m.mu.Lock()
	delete(m.rooms, id)
	m.mu.Unlock()
}
