//go:build js

package message_port

import (
	"context"
	"io"
	"runtime"
	"syscall/js"
	"unsafe"

	"github.com/pkg/errors"
)

const tinyGoPostBytes = "BLDR_TINYGO_POST_BYTES"

// MessagePort wraps a MessagePort object into a in/out Uint8Array stream.
//
// It is expected that the remote is using a MessagePortDuplex.
// Writes a null value when closing the stream.
// NOTE: This assumes we are running in a single-threaded environment!
type MessagePort struct {
	chObj      js.Value
	chPost     js.Value
	uint8Array js.Value
	onMessage  js.Func

	trig         chan struct{}
	msgs         [][]byte
	closed       bool
	onMessageSet bool
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
	s.onMessage = js.FuncOf(
		func(t js.Value, args []js.Value) any {
			if len(args) < 1 || s.closed {
				return nil
			}

			msgEvent := args[0]
			dat := msgEvent.Get("data")

			// data == null -> stream closed
			if dat.IsNull() {
				s.closed = true
				defer s.releaseOnMessage()
			} else {
				dlen := dat.Length()
				bin := make([]byte, dlen)
				js.CopyBytesToGo(bin, dat)
				s.msgs = append(s.msgs, bin)
			}

			s.wakeReader()

			return nil
		},
	)
	s.onMessageSet = true
	chObj.Set("onmessage", s.onMessage)
	chObj.Call("start")
	return s
}

// ReadMessage reads a single incoming packet from the stream.
func (s *MessagePort) ReadMessage(ctx context.Context) ([]byte, error) {
	for {
		if s.closed {
			return nil, io.EOF
		}

		if len(s.msgs) != 0 {
			nextMsg := s.msgs[0]
			copy(s.msgs, s.msgs[1:])
			s.msgs[len(s.msgs)-1] = nil
			s.msgs = s.msgs[:len(s.msgs)-1]
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
func (s *MessagePort) WriteMessage(p []byte) error {
	if runtime.Compiler == "tinygo" {
		postBytes := js.Global().Get(tinyGoPostBytes)
		if postBytes.IsUndefined() || postBytes.IsNull() || postBytes.Type() != js.TypeFunction {
			return errors.New("tinygo message port byte helper unavailable")
		}
		var ptr uint32
		if len(p) != 0 {
			ptr = uint32(uintptr(unsafe.Pointer(&p[0])))
		}
		postBytes.Invoke(s.chObj, ptr, len(p))
		return nil
	}

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
	return nil
}

// Close closes the channels.
func (s *MessagePort) Close() error {
	if s.closed {
		s.releaseOnMessage()
		s.wakeReader()
		return nil
	}

	s.closed = true
	s.releaseOnMessage()
	s.wakeReader()
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

func (s *MessagePort) wakeReader() {
	if s.trig == nil {
		return
	}

	close(s.trig)
	s.trig = nil
}

func (s *MessagePort) releaseOnMessage() {
	if !s.onMessageSet {
		return
	}

	s.chObj.Set("onmessage", js.Null())
	s.onMessage.Release()
	s.onMessageSet = false
}
