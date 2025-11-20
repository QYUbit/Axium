package defaultserializer

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

type Writer interface {
	WriteByte(b byte) error
	Write(p []byte) (n int, err error)
	WriteUvarint(x uint64) error
	WriteVarint(x int64) error
	WriteUint16(x uint16) error
	WriteUint32(x uint32) error
	WriteUint64(x uint64) error
	Reset()
	Len() int
}

type Reader interface {
	ReadByte() (byte, error)
	Read(p []byte) (n int, err error)
	ReadUvarint() (uint64, error)
	ReadVarint() (int64, error)
	ReadUint16() (uint16, error)
	ReadUint32() (uint32, error)
	ReadUint64() (uint64, error)
	Remaining() int
	Skip(n int) error
	Compact()
	Reset()
	Len() int
}

// Implements Reader & Writer
type Buffer struct {
	buf []byte
	pos int
}

func NewBuffer() *Buffer {
	return &Buffer{buf: make([]byte, 0, 256)}
}

func NewBufferFrom(data []byte) *Buffer {
	return &Buffer{buf: data, pos: 0}
}

func (b *Buffer) String() string {
	return fmt.Sprintf("Buffer[pos=%d len=%d cap=%d]", b.pos, len(b.buf), cap(b.buf))
}

func (b *Buffer) Bytes() []byte {
	return b.buf
}

func (b *Buffer) Len() int {
	return len(b.buf)
}

func (b *Buffer) GetPosition() int {
	return b.pos
}

func (b *Buffer) SetPosition(pos int) error {
	if pos < 0 || pos > len(b.buf) {
		return io.ErrUnexpectedEOF
	}
	b.pos = pos
	return nil
}

func (b *Buffer) Remaining() int {
	return len(b.buf) - b.pos
}

func (b *Buffer) Skip(n int) error {
	if b.pos+n > len(b.buf) {
		return io.ErrUnexpectedEOF
	}
	b.pos += n
	return nil
}

func (b *Buffer) Reset() {
	b.buf = b.buf[:0]
	b.pos = 0
}

func (b *Buffer) Compact() {
	if b.pos == 0 {
		return
	}
	copy(b.buf, b.buf[b.pos:])
	b.buf = b.buf[:len(b.buf)-b.pos]
	b.pos = 0
}

func (b *Buffer) WriteByte(v byte) error {
	b.buf = append(b.buf, v)
	return nil
}

func (b *Buffer) Write(p []byte) (int, error) {
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *Buffer) WriteUvarint(x uint64) error {
	b.buf = binary.AppendUvarint(b.buf, x)
	return nil
}

func (b *Buffer) WriteVarint(x int64) error {
	b.buf = binary.AppendVarint(b.buf, x)
	return nil
}

func (b *Buffer) WriteUint16(x uint16) error {
	b.buf = binary.LittleEndian.AppendUint16(b.buf, x)
	return nil
}

func (b *Buffer) WriteUint32(x uint32) error {
	b.buf = binary.LittleEndian.AppendUint32(b.buf, x)
	return nil
}

func (b *Buffer) WriteUint64(x uint64) error {
	b.buf = binary.LittleEndian.AppendUint64(b.buf, x)
	return nil
}

func (b *Buffer) ReadByte() (byte, error) {
	if b.pos >= len(b.buf) {
		return 0, io.EOF
	}
	v := b.buf[b.pos]
	b.pos++
	return v, nil
}

func (b *Buffer) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if b.pos >= len(b.buf) {
		return 0, io.EOF
	}
	n := copy(p, b.buf[b.pos:])
	b.pos += n
	return n, nil
}

func (b *Buffer) ReadUvarint() (uint64, error) {
	if b.pos >= len(b.buf) {
		return 0, io.EOF
	}
	val, n := binary.Uvarint(b.buf[b.pos:])
	if n == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	if n < 0 {
		return 0, errors.New("invalid uvarint encoding")
	}
	b.pos += n
	return val, nil
}

func (b *Buffer) ReadVarint() (int64, error) {
	if b.pos >= len(b.buf) {
		return 0, io.EOF
	}
	val, n := binary.Varint(b.buf[b.pos:])
	if n == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	if n < 0 {
		return 0, errors.New("invalid varint encoding")
	}
	b.pos += n
	return val, nil
}

func (b *Buffer) ReadUint16() (uint16, error) {
	remaining := len(b.buf) - b.pos
	if remaining == 0 {
		return 0, io.EOF
	}
	if remaining < 2 {
		return 0, io.ErrUnexpectedEOF
	}
	val := binary.LittleEndian.Uint16(b.buf[b.pos:])
	b.pos += 2
	return val, nil
}

func (b *Buffer) ReadUint32() (uint32, error) {
	remaining := len(b.buf) - b.pos
	if remaining == 0 {
		return 0, io.EOF
	}
	if remaining < 4 {
		return 0, io.ErrUnexpectedEOF
	}
	val := binary.LittleEndian.Uint32(b.buf[b.pos:])
	b.pos += 4
	return val, nil
}

func (b *Buffer) ReadUint64() (uint64, error) {
	remaining := len(b.buf) - b.pos
	if remaining == 0 {
		return 0, io.EOF
	}
	if remaining < 8 {
		return 0, io.ErrUnexpectedEOF
	}
	val := binary.LittleEndian.Uint64(b.buf[b.pos:])
	b.pos += 8
	return val, nil
}
