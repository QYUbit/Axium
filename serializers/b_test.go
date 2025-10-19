package serializers

import (
	"testing"

	"github.com/QYUbit/Axium/core"
)

func TestU(t *testing.T) {
	s := NewButilSerializer()

	data, err := s.EncodeMessage(core.Message{
		MessageAction: string(core.RoomEventAction),
		RoomEventMsg: &core.RoomEventMsg{
			RoomId:    "aaaa-aaaa-aaaa-aaaa",
			EventType: "test",
			Data:      []byte("some data"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Log(data)

	var ß core.Message
	if err := s.DecodeMessage(data, &ß); err != nil {
		t.Fatal(err)
	}

	t.Log(ß)
}
