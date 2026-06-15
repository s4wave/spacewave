package usernet

import "github.com/pkg/errors"

var (
	// ErrStackClosed is returned when the stack has been closed.
	ErrStackClosed = errors.New("usernet: stack closed")
	// ErrShortFrame is returned when a frame is too short for its header.
	ErrShortFrame = errors.New("usernet: short frame")
	// ErrUnsupportedPacket is returned when a packet cannot be handled.
	ErrUnsupportedPacket = errors.New("usernet: unsupported packet")
)
