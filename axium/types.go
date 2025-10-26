package axium

type AxiumConnection interface {
	Close(int, string)
	GetRemoteAddress() string
}

type AxiumTransport interface {
	CloseClient(string, int, string) error
	GetClientIds() []string
	Send(string, []byte, bool) error
	OnConnect(func(AxiumConnection, func(string), func(string)))
	OnDisconnect(func(string))
	OnMessage(func(string, []byte))
	OnError(func(error))
	Publish(string, []byte) error
	Subscribe(string, string) error
	Unsubscribe(string, string) error
	CreateTopic(string) error
	DeleteTopic(string) error
	GetTopicIds() []string
	GetClientIdsOfTopic(string) ([]string, error)
}

type AxiumSerializer interface {
	EncodeMessage(Message) ([]byte, error)
	DecodeMessage([]byte, *Message) error
}

type IdGenerator func() string

type MessageHandler func(session *Session, data []byte) // TODO rename message handlers to event handler
type MiddlewareHandler func(session *Session, data []byte) bool

type RoomDefinition func(*Room)

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
