package frame

import "io"

type Message struct {
	Path  string
	Data  []byte
	ReqID uint64
}

type Marshaler interface {
	Marshal(v any) (p []byte, err error)
}

type Unmarshaler interface {
	Unmarshal(v any, p []byte) (err error)
}

type Serializer interface {
	Marshaler
	Unmarshaler
}

type Encoder interface {
	Encode(w io.Writer, msg Message) error
}

type Decoder interface {
	Decode(r io.Reader) ([]Message, error)
}

type Codec interface {
	Encoder
	Decoder
}
