package plan9fs

import (
	"encoding/binary"
)

// Buffer is a read/write buffer for 9p wire format serialization.
// All values are little-endian.
type Buffer struct {
	data []byte
	off  int
	err  error
}

// NewReadBuffer creates a Buffer for reading from data.
func NewReadBuffer(data []byte) *Buffer {
	return &Buffer{data: data}
}

// NewWriteBuffer creates a Buffer for writing with initial capacity.
func NewWriteBuffer(capacity int) *Buffer {
	return &Buffer{data: make([]byte, 0, capacity)}
}

// Err returns any accumulated error.
func (b *Buffer) Err() error {
	return b.err
}

// Bytes returns the underlying byte slice.
func (b *Buffer) Bytes() []byte {
	return b.data
}

// Remaining returns the number of unread bytes.
func (b *Buffer) Remaining() int {
	r := len(b.data) - b.off
	if r < 0 {
		return 0
	}
	return r
}

// ReadU8 reads a uint8.
func (b *Buffer) ReadU8() uint8 {
	if b.err != nil || b.off+1 > len(b.data) {
		b.err = errShortRead
		return 0
	}
	v := b.data[b.off]
	b.off++
	return v
}

// ReadU16 reads a little-endian uint16.
func (b *Buffer) ReadU16() uint16 {
	if b.err != nil || b.off+2 > len(b.data) {
		b.err = errShortRead
		return 0
	}
	v := binary.LittleEndian.Uint16(b.data[b.off:])
	b.off += 2
	return v
}

// ReadU32 reads a little-endian uint32.
func (b *Buffer) ReadU32() uint32 {
	if b.err != nil || b.off+4 > len(b.data) {
		b.err = errShortRead
		return 0
	}
	v := binary.LittleEndian.Uint32(b.data[b.off:])
	b.off += 4
	return v
}

// ReadU64 reads a little-endian uint64.
func (b *Buffer) ReadU64() uint64 {
	if b.err != nil || b.off+8 > len(b.data) {
		b.err = errShortRead
		return 0
	}
	v := binary.LittleEndian.Uint64(b.data[b.off:])
	b.off += 8
	return v
}

// ReadString reads a 2-byte length-prefixed UTF-8 string.
func (b *Buffer) ReadString() string {
	n := int(b.ReadU16())
	if b.err != nil || b.off+n > len(b.data) {
		b.err = errShortRead
		return ""
	}
	s := string(b.data[b.off : b.off+n])
	b.off += n
	return s
}

// ReadBytes reads n bytes from the buffer.
func (b *Buffer) ReadBytes(n int) []byte {
	if b.err != nil || b.off+n > len(b.data) {
		b.err = errShortRead
		return nil
	}
	out := make([]byte, n)
	copy(out, b.data[b.off:b.off+n])
	b.off += n
	return out
}

// ReadQID reads a 13-byte QID.
func (b *Buffer) ReadQID() QID {
	return QID{
		Type:    b.ReadU8(),
		Version: b.ReadU32(),
		Path:    b.ReadU64(),
	}
}

// WriteU8 appends a uint8.
func (b *Buffer) WriteU8(v uint8) {
	b.data = append(b.data, v)
}

// WriteU16 appends a little-endian uint16.
func (b *Buffer) WriteU16(v uint16) {
	b.data = append(b.data, 0, 0)
	binary.LittleEndian.PutUint16(b.data[len(b.data)-2:], v)
}

// WriteU32 appends a little-endian uint32.
func (b *Buffer) WriteU32(v uint32) {
	b.data = append(b.data, 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(b.data[len(b.data)-4:], v)
}

// WriteU64 appends a little-endian uint64.
func (b *Buffer) WriteU64(v uint64) {
	b.data = append(b.data, 0, 0, 0, 0, 0, 0, 0, 0)
	binary.LittleEndian.PutUint64(b.data[len(b.data)-8:], v)
}

// WriteString appends a 2-byte length-prefixed UTF-8 string.
func (b *Buffer) WriteString(s string) {
	b.WriteU16(uint16(len(s)))
	b.data = append(b.data, s...)
}

// WriteBytes appends raw bytes.
func (b *Buffer) WriteBytes(data []byte) {
	b.data = append(b.data, data...)
}

// WriteQID appends a 13-byte QID.
func (b *Buffer) WriteQID(q QID) {
	b.WriteU8(q.Type)
	b.WriteU32(q.Version)
	b.WriteU64(q.Path)
}

// buildMessage wraps a payload with the 9p header [size:u32][type:u8][tag:u16].
func buildMessage(msgType uint8, tag uint16, payload []byte) []byte {
	size := uint32(headerSize + len(payload))
	msg := make([]byte, size)
	binary.LittleEndian.PutUint32(msg[0:], size)
	msg[4] = msgType
	binary.LittleEndian.PutUint16(msg[5:], tag)
	copy(msg[headerSize:], payload)
	return msg
}

// buildErrorResponse builds an RLERROR response.
func buildErrorResponse(tag uint16, errno uint32) []byte {
	buf := NewWriteBuffer(4)
	buf.WriteU32(errno)
	return buildMessage(RLERROR, tag, buf.Bytes())
}
