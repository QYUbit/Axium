package server

import "io"

// Message represents the wire protocol message. Data, ReqID and RoomID are optional.
type Message struct {
	Path   string
	Data   []byte
	ReqID  uint64
	RoomID string
}

type Writer interface {
	Write(w io.Writer, msg Message) error
}

type Reader interface {
	Read(r io.Reader) ([]Message, error)
}

// Codec represents a codec for the wire protocol.
type Codec interface {
	Writer
	Reader
}

type Marshaler interface {
	Marshal(v any) (p []byte, err error)
}

type Unmarshaler interface {
	Unmarshal(v any, p []byte) (err error)
}

// Serializer represents an user defined serializer.
type Serializer interface {
	Marshaler
	Unmarshaler
}

func marshalPayload(s Serializer, v any) ([]byte, error) {
	switch val := v.(type) {
	case []byte:
		return val, nil
	case string:
		return []byte(val), nil
	default:
		data, err := s.Marshal(v)
		return data, err
	}
}
