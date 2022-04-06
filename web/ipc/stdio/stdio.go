//go:build !js
// +build !js

package stdio

import (
	"io"
	"os"

	"github.com/aperturerobotics/bldr/web/ipc"
)

// IPC implements the ipc interface with stdin/stdout
type IPC struct {
	io.Reader
	io.Writer
}

// NewIPC constructs a new IPC with stdin/stdout.
func NewIPC() ipc.IPC {
	return &IPC{
		Reader: os.Stdin,
		Writer: os.Stdout,
	}
}

// Close closes the ipc stream.
func (i *IPC) Close() error {
	return nil
}

// _ is a type assertion
var _ ipc.IPC = ((*IPC)(nil))
