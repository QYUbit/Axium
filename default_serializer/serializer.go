package defaultserializer

import (
	"fmt"
	"sync"

	"github.com/QYUbit/Axium/axium"
)

var bufferPool = sync.Pool{
	New: func() any {
		return NewBuffer()
	},
}

type Serializer struct{}

func NewSerializer() *Serializer {
	return &Serializer{}
}

func (s *Serializer) EncodeMessage(msg *axium.Message) ([]byte, error) {
	buf := bufferPool.Get().(*Buffer)
	defer bufferPool.Put(buf)

	buf.Reset()

	if err := s.encode(buf, msg); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (s *Serializer) encode(w Writer, msg *axium.Message) error {
	if err := w.WriteByte(uint8(msg.Action)); err != nil {
		return err
	}

	switch msg.Action {
	case 0:
		return s.encodeServerEvent(w, msg)
	case 1:
		return s.encodeRoomEvent(w, msg)
	}

	// TODO Remove this error later
	return fmt.Errorf("unexpected action %d", msg.Action)
}

func (s *Serializer) encodeServerEvent(w Writer, msg *axium.Message) error {
	if err := DelimitedBytes.Encode(w, []byte(msg.Event)); err != nil {
		return err
	}
	return DelimitedBytes.Encode(w, msg.Data)
}

func (s *Serializer) encodeRoomEvent(w Writer, msg *axium.Message) error {
	if err := DelimitedBytes.Encode(w, []byte(msg.RoomId)); err != nil {
		return err
	}
	if err := DelimitedBytes.Encode(w, []byte(msg.Event)); err != nil {
		return err
	}
	return DelimitedBytes.Encode(w, msg.Data)
}

func (s *Serializer) DecodeMessage(data []byte) (*axium.Message, error) {
	buf := NewBufferFrom(data)
	return s.decode(buf)
}

func (s *Serializer) decode(r Reader) (*axium.Message, error) {
	action, err := r.ReadByte()
	if err != nil {
		return nil, err
	}

	switch axium.MessageAction(action) {
	case axium.ServerEventAction:
		return s.decodeServerEvent(r)
	case axium.RoomEventAction:
		return s.decodeRoomEvent(r)
	}

	return nil, fmt.Errorf("unexpected action %d", action)
}

func (s *Serializer) decodeServerEvent(r Reader) (*axium.Message, error) {
	event, err := DelimitedBytes.Decode(r)
	if err != nil {
		return nil, err
	}

	data, err := DelimitedBytes.Decode(r)
	if err != nil {
		return nil, err
	}

	msg := &axium.Message{
		Action: axium.ServerEventAction,
		Event:  string(event),
		Data:   data,
	}
	return msg, nil
}

func (s *Serializer) decodeRoomEvent(r Reader) (*axium.Message, error) {
	roomId, err := DelimitedBytes.Decode(r)
	if err != nil {
		return nil, err
	}

	event, err := DelimitedBytes.Decode(r)
	if err != nil {
		return nil, err
	}

	data, err := DelimitedBytes.Decode(r)
	if err != nil {
		return nil, err
	}

	msg := &axium.Message{
		Action: axium.RoomEventAction,
		Event:  string(event),
		Data:   data,
		RoomId: string(roomId),
	}
	return msg, nil
}
