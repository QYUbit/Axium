package defaultserializer

var DelimitedBytes = DelimitedBytesType{}

type DelimitedBytesType struct{}

func (db DelimitedBytesType) Encode(w Writer, data []byte) error {
	if err := w.WriteUvarint(uint64(len(data))); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

func (db DelimitedBytesType) Decode(r Reader) ([]byte, error) {
	dataLen, err := r.ReadUvarint()
	if err != nil {
		return nil, err
	}
	data := make([]byte, 0, dataLen)
	_, err = r.Read(data)
	return data, err
}
