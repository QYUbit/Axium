package core

type MessageAction string

const (
	RoomEventAction   MessageAction = "room_event"
	ServerEventAction MessageAction = "server_event"
)

type Message struct {
	MessageAction  string
	RoomEventMsg   *RoomEventMsg
	ServerEventMsg *ServerEventMsg
}

type RoomEventMsg struct {
	EventType string
	Data      []byte
	RoomId    string
}

type ServerEventMsg struct {
	EventType string
	Data      []byte
}
