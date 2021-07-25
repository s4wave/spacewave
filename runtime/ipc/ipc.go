package ipc

import "io"

// IPC is the common interface implemented by IPC methods.
type IPC interface {
	io.ReadWriteCloser
}
