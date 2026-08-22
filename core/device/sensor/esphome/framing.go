package esphome

import (
	"encoding/binary"
	"strconv"
)

// Preamble is the ESPHome Native API frame preamble byte.
const Preamble = 0x00

// DefaultMaxFrameSize is the maximum accepted frame payload size.
const DefaultMaxFrameSize = 1024 * 1024

// Frame is one decoded Native API frame.
type Frame struct {
	// MessageID is the Native API message identifier.
	MessageID uint32
	// Payload is the protobuf-encoded message body.
	Payload []byte
}

// ErrFrameFormat reports a malformed Native API frame.
type ErrFrameFormat struct {
	// Detail describes the malformed field.
	Detail string
}

func (e *ErrFrameFormat) Error() string {
	return "esphome frame format: " + e.Detail
}

func appendVarint(dst []byte, value uint64) []byte {
	for value >= 0x80 {
		dst = append(dst, byte(value)|0x80)
		value >>= 7
	}
	return append(dst, byte(value))
}

func decodeVarint(src []byte, offset int) (uint64, int, bool) {
	var value uint64
	var shift uint
	for i := offset; i < len(src); i++ {
		if shift >= 64 {
			return 0, 0, false
		}
		b := src[i]
		value |= uint64(b&0x7f) << shift
		if b < 0x80 {
			return value, i + 1, true
		}
		shift += 7
	}
	return 0, 0, false
}

// EncodeFrame encodes one Native API preamble, length, ID, and payload frame.
func EncodeFrame(messageID uint32, payload []byte) []byte {
	frame := make([]byte, 0, 1+binary.MaxVarintLen32*2+len(payload))
	frame = append(frame, Preamble)
	frame = appendVarint(frame, uint64(len(payload)))
	frame = appendVarint(frame, uint64(messageID))
	return append(frame, payload...)
}

// FrameDecoder decodes fragmented and coalesced Native API frames.
type FrameDecoder struct {
	maxFrameSize int
	buf          []byte
}

// NewFrameDecoder constructs a FrameDecoder with the default max frame size.
func NewFrameDecoder() *FrameDecoder {
	return &FrameDecoder{maxFrameSize: DefaultMaxFrameSize}
}

// Push decodes every complete frame in the chunk and buffers the remainder.
func (d *FrameDecoder) Push(chunk []byte) ([]Frame, error) {
	d.buf = append(d.buf, chunk...)
	var frames []Frame
	offset := 0
	for offset < len(d.buf) {
		if d.buf[offset] != Preamble {
			return nil, &ErrFrameFormat{Detail: "invalid preamble at byte " + strconv.Itoa(offset)}
		}
		length, next, ok := decodeVarint(d.buf, offset+1)
		if !ok {
			break
		}
		if length > uint64(d.maxFrameSize) {
			return nil, &ErrFrameFormat{Detail: "frame length " + strconv.FormatUint(length, 10) + " exceeds " + strconv.Itoa(d.maxFrameSize)}
		}
		messageID, idNext, ok := decodeVarint(d.buf, next)
		if !ok {
			break
		}
		end := idNext + int(length)
		if end > len(d.buf) || end < idNext {
			break
		}
		payload := make([]byte, length)
		copy(payload, d.buf[idNext:end])
		frames = append(frames, Frame{MessageID: uint32(messageID), Payload: payload})
		offset = end
	}
	d.buf = d.buf[offset:]
	return frames, nil
}

// Reset discards buffered partial frame bytes.
func (d *FrameDecoder) Reset() {
	d.buf = nil
}
