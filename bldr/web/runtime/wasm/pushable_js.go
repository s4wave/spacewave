//go:build js

package web_runtime_wasm

import (
	"io"
	"runtime"
	"sync/atomic"
	"syscall/js"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/util/jsbuf"
)

// tinyGoPushBytes is the name of the TinyGo helper global which pushes a
// copied Uint8Array into a pushable sink.
const tinyGoPushBytes = "BLDR_TINYGO_PUSH_BYTES"

// PushablePacketWriter is a PacketWriter which writes packets to a Pushable<Uint8Array>.
type PushablePacketWriter struct {
	// closed indicates whether the writer already ended the pushable.
	closed atomic.Bool
	// pushable is the JS Pushable<Uint8Array> sink.
	pushable js.Value
	// tinyGo indicates whether the TinyGo push helper is required.
	tinyGo bool
	// tinyGoPushRaw is the TinyGo push helper, valid only when tinyGo is set.
	tinyGoPushRaw js.Value
}

// NewPushablePacketWriter creates a new PushablePacketWriter.
func NewPushablePacketWriter(pushable js.Value) *PushablePacketWriter {
	w := &PushablePacketWriter{pushable: pushable}
	if runtime.Compiler == "tinygo" {
		w.tinyGo = true
		w.tinyGoPushRaw = js.Global().Get(tinyGoPushBytes)
	}
	return w
}

// WritePacket writes a packet to the remote.
func (w *PushablePacketWriter) WritePacket(pkt *srpc.Packet) error {
	// Reject writes after close.
	if w.closed.Load() {
		return io.ErrClosedPipe
	}

	// Marshal the packet and delegate the write.
	data, err := pkt.MarshalVT()
	if err != nil {
		return err
	}

	return w.WritePacketData(data)
}

// WritePacketData writes marshaled packet data to the remote.
func (w *PushablePacketWriter) WritePacketData(data []byte) error {
	// Reject writes after close.
	if w.closed.Load() {
		return io.ErrClosedPipe
	}

	// Push a JS-owned copy of the data through the TinyGo helper.
	if w.tinyGo {
		if w.tinyGoPushRaw.IsUndefined() || w.tinyGoPushRaw.IsNull() || w.tinyGoPushRaw.Type() != js.TypeFunction {
			return errors.New("tinygo push bytes helper unavailable")
		}
		arr, err := jsbuf.CopyBytesToJS(data)
		if err != nil {
			return err
		}
		if !w.tinyGoPushRaw.Invoke(w.pushable, arr).Bool() {
			return errors.New("tinygo push bytes failed")
		}
		return nil
	}

	// Push a JS-owned copy of the data directly into the sink.
	arr, err := jsbuf.CopyBytesToJS(data)
	if err != nil {
		return err
	}
	w.pushable.Get("push").Invoke(arr)
	return nil
}

// Close closes the writer.
func (w *PushablePacketWriter) Close() error {
	if !w.closed.Swap(true) {
		w.pushable.Get("end").Invoke()
	}
	return nil
}

// _ is a type assertion.
var _ srpc.PacketWriter = (*PushablePacketWriter)(nil)
