package axium

import "errors"

var (
	// Message errors
	ErrInvalidMessageAction  = errors.New("invalid message action")
	ErrMissingRoomEventMsg   = errors.New("missing room event message")
	ErrMissingServerEventMsg = errors.New("missing server event message")
	ErrMissingRoomId         = errors.New("missing room id")
	ErrMissingEventType      = errors.New("missing event type")
	ErrUnknownMessageAction  = errors.New("unknown message action")

	// Room errors
	ErrRoomNotFound      = errors.New("room not found")
	ErrRoomAlreadyExists = errors.New("room already exists")
	ErrRoomTypeNotFound  = errors.New("room type not found")
	ErrNotInRoom         = errors.New("session is not in room")
	ErrAlreadyInRoom     = errors.New("session is already in room")

	// Session errors
	ErrSessionNotFound      = errors.New("session not found")
	ErrSessionAlreadyExists = errors.New("session already exists")

	// Transport errors
	ErrTransportClosed = errors.New("transport is closed")
	ErrTopicNotFound   = errors.New("topic not found")
	ErrClientNotFound  = errors.New("client not found")

	// Serialization errors
	ErrSerializationFailed   = errors.New("serialization failed")
	ErrDeserializationFailed = errors.New("deserialization failed")
)
