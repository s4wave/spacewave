package wayland

import (
	"encoding/binary"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/plugin/wasivm/kernel"
)

// errMalformed reports a request whose arguments overrun its message.
var errMalformed = errors.New("malformed request")

// message reads the arguments of one request in order. A read past the end,
// or of a file the client did not send, records an error and returns zero, so
// a handler reads every argument and checks err once before acting.
type message struct {
	// args is the unread argument bytes.
	args []byte
	// files is the client's queue of received files, from which fd takes
	// the next file.
	files *[]*kernel.File
	// err is the first decoding error.
	err error
}

// uint reads an unsigned integer, an object id or a new id.
func (m *message) uint() uint32 {
	if len(m.args) < 4 {
		m.err = errMalformed
		return 0
	}
	v := binary.LittleEndian.Uint32(m.args)
	m.args = m.args[4:]
	return v
}

// int reads a signed integer or a fixed-point number.
func (m *message) int() int32 {
	return int32(m.uint()) //nolint:gosec // the wire carries two's complement.
}

// bytes reads a string or an array: a length and that many bytes padded to
// four bytes.
func (m *message) bytes() []byte {
	// Check the padded bytes are present.
	n := m.uint()
	padded := (uint64(n) + 3) &^ 3
	if uint64(len(m.args)) < padded {
		m.err = errMalformed
		return nil
	}
	b := m.args[:n]
	m.args = m.args[padded:]
	return b
}

// string reads a string, dropping its terminating NUL.
func (m *message) string() string {
	b := m.bytes()
	if len(b) == 0 {
		return ""
	}
	return string(b[:len(b)-1])
}

// fd takes the next file the client sent. The caller owns it.
func (m *message) fd() *kernel.File {
	if len(*m.files) == 0 {
		m.err = errors.New("request lacks its file")
		return nil
	}
	f := (*m.files)[0]
	*m.files = (*m.files)[1:]
	return f
}
