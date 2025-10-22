package serializers

import "github.com/QYUbit/Axium/axium"

type TestSerializer struct {
}

func (s *TestSerializer) EncodeMessage(msg axium.Message) ([]byte, error) {
	panic("not implemented yet")
}

func (s *TestSerializer) DecodeMessage(data []byte, dest *axium.Message) error {
	panic("not implemented yet")
}
