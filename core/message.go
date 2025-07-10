package core

import (
	butil "github.com/QYUbit/Butil/go"
)

type MessageAction uint8

const (
	RoomEventAction MessageAction = iota
)

type Message struct {
	MessageAction uint8      `butil:"message_action"`
	RoomEvent     *RoomEvent `butil:"room_event"`
}

var messageModel, _ = butil.NewModel(
	butil.Field(0, "message_action", butil.Uint8),
	butil.OptionalField(1, "room_event", butil.Reference(roomEventModel)),
)

type RoomEvent struct {
	EventType uint32 `butil:"event_type"`
	Data      []byte `butil:"data"`
	RoomId    string `butil:"room_id"`
}

var roomEventModel, _ = butil.NewModel(
	butil.Field(0, "event_type", butil.Uint32),
	butil.Field(1, "data", butil.Bytes),
	butil.Field(2, "room_id", butil.String),
)
