//go:build js
// +build js

package broadcast_channel

import (
	"context"
	"sync"
	"syscall/js"

	"github.com/aperturerobotics/bldr/web/ipc"
)

// BroadcastChannel wraps two broadcast channels for send / receive.
type BroadcastChannel struct {
	ctx context.Context

	rxCh       js.Value
	txCh       js.Value
	txChPost   js.Value
	uint8Array js.Value

	trigger chan struct{}
	mtx     sync.Mutex
	msgs    [][]byte
}

// NewBroadcastChannel builds a new BroadcastChannel send/receive pair.
func NewBroadcastChannel(ctx context.Context, txID, rxID string) *BroadcastChannel {
	global := js.Global()
	uint8ArrayCtor := global.Get("Uint8Array")

	broadcastChannelCtor := global.Get("BroadcastChannel")
	rxCh := broadcastChannelCtor.New(rxID)
	txCh := broadcastChannelCtor.New(txID)
	txChPost := txCh.Get("postMessage").Call("bind", txCh)
	s := &BroadcastChannel{
		ctx:        ctx,
		rxCh:       rxCh,
		txCh:       txCh,
		txChPost:   txChPost,
		uint8Array: uint8ArrayCtor,
		trigger:    make(chan struct{}, 1),
	}
	rxCh.Set("onmessage", js.FuncOf(
		func(t js.Value, args []js.Value) interface{} {
			if len(args) < 1 {
				return nil
			}

			// note: possibly type check to ensure Uint8Array here
			msgEvent := args[0]
			dat := msgEvent.Get("data")
			dlen := dat.Length()
			bin := make([]byte, dlen)
			js.CopyBytesToGo(bin, dat)
			// note: we cannot block here, use new goroutine
			go s.handleMessage(bin)
			return nil
		},
	))
	return s
}

// handleMessage handles a message on the bus.
func (s *BroadcastChannel) handleMessage(dat []byte) {
	s.mtx.Lock()
	s.msgs = append(s.msgs, dat)
	select {
	case s.trigger <- struct{}{}:
	default:
	}
	s.mtx.Unlock()
}

// Read reads a message from the stream.
func (s *BroadcastChannel) Read(p []byte) (n int, err error) {
	for {
		s.mtx.Lock()
		if len(s.msgs) != 0 {
			msg := s.msgs[0]
			msgLen := len(msg)
			if msgLen < len(p) {
				p = p[:msgLen]
			}
			copy(p, msg)
			s.msgs[0] = msg[len(p):]
			if len(s.msgs[0]) == 0 {
				s.msgs = s.msgs[1:]
			}
			msgCount := len(s.msgs)
			s.mtx.Unlock()
			if len(p) != 0 && msgCount != 0 {
				select {
				case s.trigger <- struct{}{}:
				default:
				}
			}
			return len(p), nil
		} else {
			s.mtx.Unlock()
		}

		select {
		case <-s.ctx.Done():
			return 0, s.ctx.Err()
		case <-s.trigger:
		}
	}
}

// Write writes a message to the stream.
func (s *BroadcastChannel) Write(p []byte) (n int, err error) {
	a := s.uint8Array.New(len(p))
	js.CopyBytesToJS(a, p)
	s.txChPost.Invoke(a)
	return len(p), nil
}

// Close closes the channels.
func (s *BroadcastChannel) Close() error {
	return nil
}

// _ is a type assertion
var _ ipc.IPC = ((*BroadcastChannel)(nil))
