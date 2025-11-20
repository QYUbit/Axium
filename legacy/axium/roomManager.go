package axium

import (
	"fmt"
	"sync"
)

type IRoomManager interface {
	DefineRoom(roomType string, definition RoomDefinition)
	GetRoom(roomId string) (*Room, bool)
	GetRooms() []*Room
	GetRoomIds() []string
	DestroyRoom(roomId string) error
}

type RoomManager struct {
	rooms            map[string]*Room
	roomMu           sync.RWMutex
	roomDefinitions  map[string]RoomDefinition
	roomDefinitionMu sync.RWMutex
}

func (rm *RoomManager) DestroyRoom(roomId string) error {
	rm.roomMu.Lock()
	room, exists := rm.rooms[roomId]
	if exists {
		delete(rm.rooms, roomId)
	}
	rm.roomMu.Unlock()

	if !exists {
		return fmt.Errorf("room %s does not exist", roomId)
	}

	return room.destroy()
}

func (rm *RoomManager) getRoom(id string) (*Room, error) {
	rm.roomMu.RLock()
	room, exists := rm.rooms[id]
	rm.roomMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("room %s does not exist\n", id)
	}
	return room, nil
}

func (rm *RoomManager) setRooom(id string, room *Room) {
	rm.roomMu.Lock()
	rm.rooms[id] = room
	rm.roomMu.Unlock()
}

func (rm *RoomManager) GetRoom(roomId string) (*Room, bool) {
	rm.roomMu.RLock()
	defer rm.roomMu.RUnlock()
	room, exists := rm.rooms[roomId]
	return room, exists
}

func (rm *RoomManager) GetRooms() []*Room {
	rm.roomMu.RLock()
	defer rm.roomMu.RUnlock()
	rooms := make([]*Room, 0, len(rm.rooms))
	for _, room := range rm.rooms {
		rooms = append(rooms, room)
	}
	return rooms
}

func (rm *RoomManager) GetRoomIds() []string {
	rm.roomMu.RLock()
	defer rm.roomMu.RUnlock()
	ids := make([]string, 0, len(rm.rooms))
	for id := range rm.rooms {
		ids = append(ids, id)
	}
	return ids
}

func (rm *RoomManager) DefineRoom(typ string, room RoomDefinition) {
	rm.roomDefinitionMu.Lock()
	rm.roomDefinitions[typ] = room
	rm.roomDefinitionMu.Unlock()
}
