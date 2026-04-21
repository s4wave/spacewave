//go:build js
// +build js

package message_port

import (
	"context"
	"io"
	"syscall/js"

	"github.com/aperturerobotics/util/cqueue"
)

// MessagePort wraps a MessagePort object into a in/out Uint8Array stream.
//
// It is expected that the remote is using a MessagePortDuplex.
// Writes a null value when closing the stream.
// NOTE: This assumes we are running in a single-threaded environment!
type MessagePort struct {
	chObj      js.Value
	chPost     js.Value
	uint8Array js.Value

	trig   chan struct{}
	msgs   cqueue.AtomicLIFO[[]byte]
	closed bool
}

// NewMessagePort builds a new MessagePort send/receive pair.
func NewMessagePort(chObj js.Value) *MessagePort {
	global := js.Global()
	uint8ArrayCtor := global.Get("Uint8Array")
	chPostMsg := chObj.Get("postMessage")
	chPost := chPostMsg.Call("bind", chObj)
	s := &MessagePort{
		chObj:      chObj,
		chPost:     chPost,
		uint8Array: uint8ArrayCtor,
	}
	chObj.Set("onmessage", js.FuncOf(
		func(t js.Value, args []js.Value) interface{} {
			if len(args) < 1 || s.closed {
				return nil
			}

			msgEvent := args[0]
			dat := msgEvent.Get("data")

			// data == null -> stream closed
			if dat.IsNull() {
				s.closed = true
			} else {
				dlen := dat.Length()
				bin := make([]byte, dlen)
				js.CopyBytesToGo(bin, dat)
				// note: we cannot block here, use atomic ops
				s.msgs.Push(bin)
			}

			if s.trig != nil {
				close(s.trig)
				s.trig = nil
			}

			return nil
		},
	))
	chObj.Call("start")
	return s
}

// ReadMessage reads a single incoming packet from the stream.
func (s *MessagePort) ReadMessage(ctx context.Context) ([]byte, error) {
	for {
		if s.closed {
			return nil, io.EOF
		}

		nextMsg := s.msgs.Pop()
		if len(nextMsg) != 0 {
			return nextMsg, nil
		}

		trig := s.trig
		if trig == nil {
			trig = make(chan struct{})
			s.trig = trig
		}

		select {
		case <-ctx.Done():
			return nil, context.Canceled
		case <-trig:
		}
	}
}

// WriteMessage writes a message to the stream.
func (s *MessagePort) WriteMessage(p []byte) {
	a := s.uint8Array.New(len(p))
	js.CopyBytesToJS(a, p)
	if s.chPost.IsUndefined() || s.chPost.IsNull() || s.chPost.Type() != js.TypeFunction {
		panic("message port postMessage unavailable")
	}
	defer func() {
		if e := recover(); e != nil {
			panic("message port postMessage invoke failed")
		}
	}()
	s.chPost.Invoke(a)
}

// Close closes the channels.
func (s *MessagePort) Close() error {
	s.closed = true
	if s.chPost.IsUndefined() || s.chPost.IsNull() || s.chPost.Type() != js.TypeFunction {
		panic("message port postMessage unavailable during close")
	}
	defer func() {
		if e := recover(); e != nil {
			panic("message port close invoke failed")
		}
	}()
	s.chPost.Invoke(js.Null())
	s.chObj.Call("close")
	return nil
}
