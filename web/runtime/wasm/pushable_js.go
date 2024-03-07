//go:build js

package web_runtime_wasm

import (
	"io"
	"sync/atomic"
	"syscall/js"

	"github.com/aperturerobotics/starpc/srpc"
)

// PushablePacketWriter is a PacketWriter which writes packets to a Pushable<Uint8Array>.
type PushablePacketWriter struct {
	closed   atomic.Bool
	pushable js.Value
}

// NewPushablePacketWriter creates a new PushablePacketWriter.
func NewPushablePacketWriter(pushable js.Value) *PushablePacketWriter {
	return &PushablePacketWriter{pushable: pushable}
}

// WritePacket writes a packet to the remote.
func (w *PushablePacketWriter) WritePacket(pkt *srpc.Packet) error {
	if w.closed.Load() {
		return io.ErrClosedPipe
	}

	data, err := pkt.MarshalVT()
	if err != nil {
		return err
	}

	a := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(a, data)
	w.pushable.Call("push", a)
	return nil
}

// Close closes the writer.
func (w *PushablePacketWriter) Close() error {
	if !w.closed.Swap(true) {
		w.pushable.Call("end")
	}
	return nil
}

// _ is a type assertion
var _ srpc.PacketWriter = (*PushablePacketWriter)(nil)
