package gstencoder

import "github.com/pkg/errors"

const (
	// Executable is the helper looked up on the Device's local PATH.
	Executable = "spacewave-gst-encoder"
	// MaxPacketSize bounds each access unit and its headers at 16 MiB.
	MaxPacketSize = 16 << 20
)

// ErrEncoderFailed reports an unusable or exited encoder.
var ErrEncoderFailed = errors.New("display encoder stopped. Check spacewave-gst-encoder and restart the display node")
