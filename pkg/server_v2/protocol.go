package s2

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
)

type Message struct {
	IsRoom bool
	Room   string
	Path   string
	Data   []byte
}

type msgDispatcher func(ctx context.Context, s *Session, msg Message)

type MessageProtocol interface {
	ReadMessage(dest *Message, r io.Reader) error
	WriteMessage(w io.Writer, src Message) error
}

// Simple, but working binary codec
type DefaultProtocol struct{}

func NewDefaultProtocol() DefaultProtocol {
	return DefaultProtocol{}
}

func (DefaultProtocol) ReadMessage(dest *Message, r io.Reader) error {
	pathLen, err := readUvarint(r)
	if err != nil {
		return err
	}

	path := make([]byte, pathLen)
	if _, err := r.Read(path); err != nil {
		return err
	}

	roomLen, err := readUvarint(r)
	if err != nil {
		return err
	}

	room := make([]byte, roomLen)
	if _, err := r.Read(room); err != nil {
		return err
	}

	payloadLen, err := readUvarint(r)
	if err != nil {
		return err
	}

	payload := make([]byte, payloadLen)
	if _, err := r.Read(payload); err != nil {
		return err
	}

	dest.Path = string(path)
	dest.IsRoom = roomLen > 0
	dest.Room = string(room)
	dest.Data = payload

	return nil
}

func (DefaultProtocol) WriteMessage(w io.Writer, src Message) error {
	if _, err := writeUvarint(w, uint64(len(src.Path))); err != nil {
		return err
	}
	if _, err := w.Write([]byte(src.Path)); err != nil {
		return err
	}

	if _, err := writeUvarint(w, uint64(len(src.Room))); err != nil {
		return err
	}
	if _, err := w.Write([]byte(src.Room)); err != nil {
		return err
	}

	if _, err := writeUvarint(w, uint64(len(src.Data))); err != nil {
		return err
	}
	if _, err := w.Write(src.Data); err != nil {
		return err
	}
	return nil
}

func writeUvarint(w io.Writer, v uint64) (int, error) {
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], v)

	written := 0
	for written < n {
		m, err := w.Write(buf[written:n])
		written += m
		if err != nil {
			return written, err
		}
		if m == 0 {
			return written, io.ErrUnexpectedEOF
		}
	}
	return written, nil
}

func readUvarint(r io.Reader) (uint64, error) {
	br, ok := r.(io.ByteReader)
	if !ok {
		br = bufio.NewReader(r)
	}
	return binary.ReadUvarint(br)
}
