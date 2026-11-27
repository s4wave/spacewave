package wayland

import (
	"encoding/binary"
)

// event encodes one event: a header recording the object and opcode, then the
// arguments, each padded to four bytes.
type event struct {
	// buf holds the header and the arguments written so far.
	buf []byte
}

// newEvent starts an event from object id with opcode op.
func newEvent(id uint32, op uint16) *event {
	e := &event{buf: make([]byte, 8, 64)}
	binary.LittleEndian.PutUint32(e.buf, id)
	binary.LittleEndian.PutUint16(e.buf[4:], op)
	return e
}

// uint appends an unsigned integer or an object id.
func (e *event) uint(v uint32) *event {
	e.buf = binary.LittleEndian.AppendUint32(e.buf, v)
	return e
}

// int appends a signed integer.
func (e *event) int(v int32) *event {
	return e.uint(uint32(v)) //nolint:gosec // the wire carries two's complement.
}

// fixed appends v as a 24.8 fixed-point number.
func (e *event) fixed(v int32) *event {
	return e.int(v * 256)
}

// array appends an array: its length, then its bytes padded to four bytes.
func (e *event) array(b []byte) *event {
	e.uint(uint32(len(b))) //nolint:gosec // events are far below 4 GiB.
	e.buf = append(e.buf, b...)
	for len(e.buf)%4 != 0 {
		e.buf = append(e.buf, 0)
	}
	return e
}

// string appends a string with its terminating NUL.
func (e *event) string(s string) *event {
	return e.array(append([]byte(s), 0))
}

// uints appends an array of unsigned integers.
func (e *event) uints(vs ...uint32) *event {
	b := make([]byte, 0, 4*len(vs))
	for _, v := range vs {
		b = binary.LittleEndian.AppendUint32(b, v)
	}
	return e.array(b)
}

// bytes returns the encoded event, with its size in the header.
func (e *event) bytes() []byte {
	binary.LittleEndian.PutUint16(e.buf[6:], uint16(len(e.buf))) //nolint:gosec // events are below 64 KiB.
	return e.buf
}
