package serializers

import (
	"github.com/QYUbit/Axium/core"
	butil "github.com/QYUbit/Butil/go"
)

// Implements core.AxiumSerializer
type ButilSerializer struct {
	messageModel, roomModel, reconnectModel *butil.Model
}

func NewButilSerializer() *ButilSerializer {
	var roomEventModel, _ = butil.NewModel(
		butil.Field(0, "event_type", butil.String),
		butil.Field(1, "data", butil.Bytes),
		butil.Field(2, "room_id", butil.String),
	)

	var reconnectModel, _ = butil.NewModel(
		butil.Field(0, "session_id", butil.String),
	)

	var messageModel, _ = butil.NewModel(
		butil.Field(0, "message_action", butil.String),
		butil.OptionalField(1, "room_event_msg", butil.Reference(roomEventModel)),
		butil.OptionalField(2, "reconnect_msg", butil.Reference(reconnectModel)),
	)

	return &ButilSerializer{
		messageModel:   messageModel,
		roomModel:      roomEventModel,
		reconnectModel: reconnectModel,
	}
}

func (s *ButilSerializer) EncodeMessage(msg core.Message) ([]byte, error) {
	return s.messageModel.Encode(msg)
}

func (s *ButilSerializer) DecodeMessage(data []byte, dest *core.Message) error {
	return s.messageModel.Decode(data, dest)
}
