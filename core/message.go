package core

type MessageAction uint8

const (
	RoomEventAction MessageAction = iota
)

type Message struct {
	MessageAction uint8      `butil:"message_action"`
	RoomEvent     *RoomEvent `butil:"room_event"`
}

type RoomEvent struct {
	EventType uint32 `butil:"event_type"`
	Data      []byte `butil:"data"`
	RoomId    string `butil:"room_id"`
}
