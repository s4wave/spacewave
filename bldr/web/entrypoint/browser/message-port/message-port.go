//go:build js

package message_port

import (
	"context"
	"errors"
	"io"
	"runtime"
	"syscall/js"

	"github.com/s4wave/spacewave/db/util/jsbuf"
)

const tinyGoPostBytes = "BLDR_TINYGO_POST_BYTES"

// MessagePort wraps a MessagePort object into a in/out Uint8Array stream.
//
// It is expected that the remote is using a MessagePortDuplex.
// Writes a null value when closing the stream.
// NOTE: MessagePort assumes a single-threaded environment.
type MessagePort struct {
	// chObj is the physical JavaScript endpoint. chPost is its bound
	// sender.
	chObj  js.Value
	chPost js.Value
	// uint8Array constructs outgoing byte buffers.
	uint8Array js.Value
	// onMessage receives ordered bytes and remote write EOF.
	onMessage js.Func

	// trig wakes a reader when a message or read closure arrives.
	trig chan struct{}
	// msgs retains received packets until ReadMessage consumes them.
	msgs [][]byte
	// readClosed and writeClosed track the independent stream directions.
	readClosed  bool
	writeClosed bool
	// closed records full endpoint disposal, including after remote EOF.
	closed bool
	// onMessageSet records ownership of the JavaScript callback.
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
			if len(args) < 1 || s.readClosed {
				return nil
			}

			msgEvent := args[0]
			dat := msgEvent.Get("data")

			// A null message closes the remote write direction.
			if dat.IsNull() {
				s.readClosed = true
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
		if len(s.msgs) != 0 {
			nextMsg := s.msgs[0]
			copy(s.msgs, s.msgs[1:])
			s.msgs[len(s.msgs)-1] = nil
			s.msgs = s.msgs[:len(s.msgs)-1]
			return nextMsg, nil
		}

		if s.readClosed {
			return nil, io.EOF
		}

		trig := s.trig
		if trig == nil {
			trig = make(chan struct{})
			s.trig = trig
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-trig:
		}
	}
}

// WriteMessage writes a message to the stream.
func (s *MessagePort) WriteMessage(p []byte) error {
	if s.writeClosed {
		return errors.New("message port write side closed")
	}

	if runtime.Compiler == "tinygo" {
		postBytes := js.Global().Get(tinyGoPostBytes)
		if postBytes.IsUndefined() || postBytes.IsNull() || postBytes.Type() != js.TypeFunction {
			return errors.New("tinygo message port byte helper unavailable")
		}
		arr, err := jsbuf.CopyBytesToJS(p)
		if err != nil {
			return err
		}
		if !postBytes.Invoke(s.chObj, arr).Bool() {
			return errors.New("tinygo message port postMessage failed")
		}
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

// CloseWrite closes the outbound side while preserving inbound reads.
func (s *MessagePort) CloseWrite() error {
	if s.writeClosed {
		return nil
	}

	s.writeClosed = true
	if s.chPost.IsUndefined() || s.chPost.IsNull() || s.chPost.Type() != js.TypeFunction {
		panic("message port postMessage unavailable during close")
	}
	defer func() {
		if e := recover(); e != nil {
			panic("message port close invoke failed")
		}
	}()
	s.chPost.Invoke(js.Null())
	return nil
}

// Close closes both sides of the channel.
func (s *MessagePort) Close() error {
	if s.closed {
		return nil
	}

	s.closed = true
	s.readClosed = true
	s.releaseOnMessage()
	s.wakeReader()
	defer s.chObj.Call("close")
	return s.CloseWrite()
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
