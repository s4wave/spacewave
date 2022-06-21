//go:build js
// +build js

package broadcast_channel

import (
	"context"
	"errors"
	"sync"
	"syscall/js"

	"github.com/aperturerobotics/bldr/web/ipc"
)

// MessagePort wraps a MessagePort object into a in/out Uint8Array stream.
type MessagePort struct {
	ctx context.Context

	chObj      js.Value
	chPost     js.Value
	uint8Array js.Value

	trigger chan struct{}
	mtx     sync.Mutex
	msgs    [][]byte
}

// NewMessagePort builds a new MessagePort send/receive pair.
func NewMessagePort(ctx context.Context, chObj js.Value) (*MessagePort, error) {
	global := js.Global()
	uint8ArrayCtor := global.Get("Uint8Array")

	chPostMsg := chObj.Get("postMessage")
	if chPostMsg.IsUndefined() {
		return nil, errors.New("message port: postMessage is undefined")
	}
	chPost := chPostMsg.Call("bind", chObj)
	s := &MessagePort{
		ctx:        ctx,
		chObj:      chObj,
		chPost:     chPost,
		uint8Array: uint8ArrayCtor,
		trigger:    make(chan struct{}, 1),
	}
	chObj.Set("onmessage", js.FuncOf(
		func(t js.Value, args []js.Value) interface{} {
			if len(args) < 1 {
				return nil
			}

			// note: possibly type check to ensure Uint8Array here
			msgEvent := args[0]
			dat := msgEvent.Get("data")
			// TODO remove
			// global.Get("console").Call("log", "Go: rx", dat)
			dlen := dat.Length()
			bin := make([]byte, dlen)
			js.CopyBytesToGo(bin, dat)
			// note: we cannot block here, use new goroutine
			go s.handleMessage(bin)
			return nil
		},
	))
	chObj.Call("start")
	return s, nil
}

// handleMessage handles a message on the bus.
func (s *MessagePort) handleMessage(dat []byte) {
	s.mtx.Lock()
	s.msgs = append(s.msgs, dat)
	select {
	case s.trigger <- struct{}{}:
	default:
	}
	s.mtx.Unlock()
}

// Read reads a message from the stream.
func (s *MessagePort) Read(p []byte) (n int, err error) {
	for {
		s.mtx.Lock()
		if len(s.msgs) != 0 {
			msg := s.msgs[0]
			msgLen := len(msg)
			if msgLen < len(p) {
				p = p[:msgLen]
			}
			copy(p, msg)
			msg = msg[len(p):]
			if len(msg) == 0 {
				s.msgs = s.msgs[1:]
			} else {
				s.msgs[0] = msg
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
func (s *MessagePort) Write(p []byte) (n int, err error) {
	a := s.uint8Array.New(len(p))
	js.CopyBytesToJS(a, p)
	// js.Global().Get("console").Call("log", "Go: tx", a)
	s.chPost.Invoke(a)
	// TODO remove
	return len(p), nil
}

// Close closes the channels.
func (s *MessagePort) Close() error {
	return nil
}

// _ is a type assertion
var _ ipc.IPC = ((*MessagePort)(nil))
