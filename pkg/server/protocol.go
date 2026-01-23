package server

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

type Writer interface {
	Write(w io.Writer, msg Message) error
}

type Reader interface {
	Read(r io.Reader) ([]Message, error)
}

type Codec interface {
	Writer
	Reader
}
