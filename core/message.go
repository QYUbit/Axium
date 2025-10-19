package core

type MessageAction string

const (
	RoomEventAction   MessageAction = "room_event"
	ReconnectAction   MessageAction = "reconnect"
	AuthAction        MessageAction = "auth"
	ServerEventAction MessageAction = "server_event"
)

type Message struct {
	MessageAction  string
	RoomEventMsg   *RoomEventMsg
	ReconnectMsg   *ReconnectMsg
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

type ReconnectMsg struct {
	SessionId string
}

type AuthMethod string

const (
	GuestAuthMethod AuthMethod = "guest"
)

type AuthMsg struct {
	Method AuthMethod
}
