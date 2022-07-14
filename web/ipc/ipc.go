package ipc

import (
	"github.com/libp2p/go-libp2p-core/network"
)

// IPC is the common interface implemented by IPC methods.
type IPC = network.MuxedConn
