package serializers

import (
	"github.com/QYUbit/Axium/core"
	butil "github.com/QYUbit/Butil/go"
)

// Implements core.AxiumSerializer
type ButilSerializer struct {
	messageModel, roomModel *butil.Model
}

func NewButilSerializer() *ButilSerializer {
	var roomEventModel, _ = butil.NewModel(
		butil.Field(0, "event_type", butil.String),
		butil.Field(1, "data", butil.Bytes),
		butil.Field(2, "room_id", butil.String),
	)

	var messageModel, _ = butil.NewModel(
		butil.Field(0, "message_action", butil.Uint8),
		butil.OptionalField(1, "room_event", butil.Reference(roomEventModel)),
	)

	return &ButilSerializer{
		messageModel: messageModel,
		roomModel:    roomEventModel,
	}
}

func (s *ButilSerializer) EncodeMessage(msg core.Message) ([]byte, error) {
	return s.messageModel.Encode(msg)
}

func (s *ButilSerializer) DecodeMessage(data []byte, dest *core.Message) error {
	return s.messageModel.Decode(data, dest)
}
