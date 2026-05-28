//go:build js

package web_runtime_wasm

import (
	"bytes"
	"context"
	"io"
	"runtime"
	"syscall/js"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
)

type openStreamHarness struct {
	value     js.Value
	ready     chan struct{}
	delivered chan struct{}
	pushed    chan struct{}
	ended     chan struct{}
}

func (h *openStreamHarness) Get(name string) js.Value {
	return h.value.Get(name)
}

func (h *openStreamHarness) Call(name string, args ...any) js.Value {
	return h.value.Call(name, args...)
}

type acceptStreamHarness struct {
	value       js.Value
	callbackSet chan struct{}
	started     chan struct{}
	posted      chan struct{}
	closed      chan struct{}
}

func (h *acceptStreamHarness) Get(name string) js.Value {
	return h.value.Get(name)
}

func (h *acceptStreamHarness) Call(name string, args ...any) js.Value {
	return h.value.Call(name, args...)
}

func TestOpenPushableStreamBuffersBeforeJSStreamResolves(t *testing.T) {
	harness := newOpenStreamHarness(t)
	writer := startOpenStreamHarness(
		t,
		harness,
		func(data []byte) error { return nil },
		func(closeErr error) {},
	)

	pkt := srpc.NewCallStartPacket("service", "method", []byte("first"), false)
	if err := writer.WritePacket(pkt); err != nil {
		t.Fatalf("write queued packet: %v", err)
	}
	if got := harness.Get("pushable").Get("messages").Length(); got != 0 {
		t.Fatalf("packet pushed before stream resolved: %d", got)
	}

	harness.Call("resolveOpen")
	waitForSignal(t, harness.pushed, "pushed packet")
	if got := harness.Get("pushable").Get("messages").Length(); got != 1 {
		t.Fatalf("pushed packets = %d want 1", got)
	}
	gotData := copyJSBytes(harness.Get("pushable").Get("messages").Index(0))
	wantData, err := pkt.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotData, wantData) {
		t.Fatalf("pushed packet mismatch: got %v want %v", gotData, wantData)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close resolved writer: %v", err)
	}
	waitForSignal(t, harness.ended, "pushable end")
}

func TestOpenPushableStreamCloseBeforeJSStreamResolves(t *testing.T) {
	harness := newOpenStreamHarness(t)
	writer := startOpenStreamHarness(
		t,
		harness,
		func(data []byte) error { return nil },
		func(closeErr error) {},
	)

	if err := writer.Close(); err != nil {
		t.Fatalf("close pending writer: %v", err)
	}
	harness.Call("resolveOpen")
	waitForSignal(t, harness.ended, "pushable end")
	if got := harness.Get("pushable").Get("endCalls").Int(); got != 1 {
		t.Fatalf("pushable end calls = %d want 1", got)
	}
	if err := writer.WritePacket(srpc.NewCallCancelPacket()); err != io.ErrClosedPipe {
		t.Fatalf("write after close = %v want %v", err, io.ErrClosedPipe)
	}
}

func TestOpenPushableStreamRejectFailsPendingWriter(t *testing.T) {
	harness := newOpenStreamHarness(t)
	closeErrCh := make(chan error, 1)
	writer := startOpenStreamHarness(
		t,
		harness,
		func(data []byte) error { return nil },
		func(closeErr error) { closeErrCh <- closeErr },
	)
	if err := writer.WritePacket(srpc.NewCallCancelPacket()); err != nil {
		t.Fatalf("write queued packet: %v", err)
	}

	harness.Call("rejectOpen", "boom")
	closeErr := waitForError(t, closeErrCh, "close error")
	if closeErr == nil || closeErr.Error() != "boom" {
		t.Fatalf("close error = %v want boom", closeErr)
	}
	if err := writer.WritePacket(srpc.NewCallCancelPacket()); err == nil || err.Error() != "boom" {
		t.Fatalf("write after reject = %v want boom", err)
	}
}

func TestOpenPushableStreamSerializesMessageHandler(t *testing.T) {
	harness := newOpenStreamHarness(t)
	started := make(chan []byte, 2)
	releaseFirst := make(chan struct{})
	writer := startOpenStreamHarness(
		t,
		harness,
		func(data []byte) error {
			started <- append([]byte(nil), data...)
			if len(data) == 1 && data[0] == 1 {
				<-releaseFirst
			}
			return nil
		},
		func(closeErr error) {},
	)

	harness.Call("resolveOpen")
	harness.Call("deliver", jsUint8Array([]byte{1}))
	if msg := waitForMessage(t, started, "first message"); !bytes.Equal(msg, []byte{1}) {
		t.Fatalf("first message = %v want [1]", msg)
	}
	waitForSignal(t, harness.delivered, "first delivered callback")

	harness.Call("deliver", jsUint8Array([]byte{2}))
	waitForSignal(t, harness.delivered, "second delivered callback")
	if msg, ok := receiveMessage(started); ok {
		t.Fatalf("second message handler started before first returned: %v", msg)
	}

	close(releaseFirst)
	if msg := waitForMessage(t, started, "second message"); !bytes.Equal(msg, []byte{2}) {
		t.Fatalf("second message = %v want [2]", msg)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close resolved writer: %v", err)
	}
	waitForSignal(t, harness.ended, "pushable end")
}

func TestSerialPacketDataHandlerAcknowledgesAfterMessageHandler(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	handled := make(chan error, 1)
	handler := newSerialPacketDataHandler(
		func(data []byte) error {
			if !bytes.Equal(data, []byte{1}) {
				t.Fatalf("handler data = %v want [1]", data)
			}
			signal(started)
			<-release
			return nil
		},
		func(closeErr error) {},
		func() {},
	)

	handler.HandleWithResult([]byte{1}, func(err error) {
		handled <- err
	})
	waitForSignal(t, started, "handler start")
	if err, ok := receiveError(handled); ok {
		t.Fatalf("packet acknowledged before handler returned: %v", err)
	}

	close(release)
	if err := waitForError(t, handled, "packet handled"); err != nil {
		t.Fatalf("packet handled error = %v", err)
	}
	handler.Close(nil)
}

func TestSerialPacketDataHandlerFailsQueuedAcknowledgements(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	firstHandled := make(chan error, 1)
	secondHandled := make(chan error, 1)
	handler := newSerialPacketDataHandler(
		func(data []byte) error {
			signal(started)
			<-release
			if bytes.Equal(data, []byte{1}) {
				return io.ErrUnexpectedEOF
			}
			return nil
		},
		func(closeErr error) {},
		func() {},
	)

	handler.HandleWithResult([]byte{1}, func(err error) {
		firstHandled <- err
	})
	waitForSignal(t, started, "first handler start")
	handler.HandleWithResult([]byte{2}, func(err error) {
		secondHandled <- err
	})

	close(release)
	if err := waitForError(t, firstHandled, "first packet handled"); err != io.ErrUnexpectedEOF {
		t.Fatalf("first packet error = %v want %v", err, io.ErrUnexpectedEOF)
	}
	if err := waitForError(t, secondHandled, "second packet handled"); err != io.ErrUnexpectedEOF {
		t.Fatalf("second packet error = %v want %v", err, io.ErrUnexpectedEOF)
	}
}

func TestSetAcceptStreamsWrapsProvidedMessagePort(t *testing.T) {
	harness := newAcceptStreamHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := &WasmPluginIo{setAcceptStreamName: "testSetAcceptStream"}
	js.Global().Set(p.setAcceptStreamName, harness.Get("setAcceptStream"))
	defer js.Global().Set(p.setAcceptStreamName, js.Undefined())

	p.SetAcceptStreams(ctx, srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
		return false, nil
	}))

	waitForSignal(t, harness.callbackSet, "accept stream callback")
	harness.Call("accept")
	waitForSignal(t, harness.started, "message port start")
	if got := harness.Get("port").Get("postCalls").Length(); got != 0 {
		t.Fatalf("post calls before close = %d want 0", got)
	}

	cancel()
	waitForSignal(t, harness.posted, "message port close post")
	waitForSignal(t, harness.closed, "message port close")
	if got := harness.Get("port").Get("postCalls").Length(); got != 1 {
		t.Fatalf("post calls after close = %d want 1", got)
	}
	if got := harness.Get("port").Get("postCalls").Index(0); !got.IsNull() {
		t.Fatalf("close post = %v want null", got)
	}
}

func copyJSBytes(value js.Value) []byte {
	data := make([]byte, value.Length())
	for i := range data {
		data[i] = byte(value.Index(i).Int())
	}
	return data
}

func startOpenStreamHarness(
	t *testing.T,
	harness *openStreamHarness,
	msgHandler srpc.PacketDataHandler,
	closeHandler srpc.CloseHandler,
) srpc.PacketWriter {
	t.Helper()
	packetCallbacks := &jsStreamPacketCallbacks{}
	writer := newDeferredPushablePacketWriter(packetCallbacks.Release)
	closeErrCh := make(chan error, 1)
	openPushableStream(
		context.Background(),
		harness.Get("openStream"),
		msgHandler,
		func(err error) {
			closeErrCh <- err
			closeHandler(err)
		},
		writer,
		packetCallbacks,
	)
	runtime.Gosched()
	select {
	case <-harness.ready:
	case err := <-closeErrCh:
		t.Fatalf("open stream failed before harness ready: %v", err)
	}
	return writer
}

func jsUint8Array(data []byte) js.Value {
	arr := js.Global().Get("Uint8Array").New(len(data))
	for i, b := range data {
		arr.SetIndex(i, int(b))
	}
	return arr
}

func waitForMessage(t *testing.T, ch <-chan []byte, name string) []byte {
	t.Helper()
	runtime.Gosched()
	return <-ch
}

func receiveMessage(ch <-chan []byte) ([]byte, bool) {
	select {
	case msg := <-ch:
		return msg, true
	default:
		return nil, false
	}
}

func receiveError(ch <-chan error) (error, bool) {
	select {
	case err := <-ch:
		return err, true
	default:
		return nil, false
	}
}

func waitForSignal(t *testing.T, ch <-chan struct{}, name string) {
	t.Helper()
	runtime.Gosched()
	<-ch
}

func waitForError(t *testing.T, ch <-chan error, name string) error {
	t.Helper()
	runtime.Gosched()
	return <-ch
}

func signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func installTinyGoTestHelpers(t *testing.T) {
	t.Helper()

	global := js.Global()
	prevNewBytes := global.Get("BLDR_TINYGO_NEW_BYTES")
	prevPushBytes := global.Get(tinyGoPushBytes)
	prevPostBytes := global.Get("BLDR_TINYGO_POST_BYTES")
	newBytes := js.FuncOf(func(this js.Value, args []js.Value) any {
		return js.Global().Get("Uint8Array").New(args[0].Int())
	})
	pushBytes := js.FuncOf(func(this js.Value, args []js.Value) any {
		sink := args[0]
		sink.Call("push", copyUint8Array(args[1]))
		return true
	})
	postBytes := js.FuncOf(func(this js.Value, args []js.Value) any {
		port := args[0]
		port.Call("postMessage", copyUint8Array(args[1]))
		return true
	})
	global.Set("BLDR_TINYGO_NEW_BYTES", newBytes)
	global.Set(tinyGoPushBytes, pushBytes)
	global.Set("BLDR_TINYGO_POST_BYTES", postBytes)
	t.Cleanup(func() {
		global.Set("BLDR_TINYGO_NEW_BYTES", prevNewBytes)
		global.Set(tinyGoPushBytes, prevPushBytes)
		global.Set("BLDR_TINYGO_POST_BYTES", prevPostBytes)
		newBytes.Release()
		pushBytes.Release()
		postBytes.Release()
	})
}

func copyUint8Array(value js.Value) js.Value {
	copy := js.Global().Get("Uint8Array").New(value.Get("byteLength").Int())
	copy.Call("set", value)
	return copy
}

func newOpenStreamHarness(t *testing.T) *openStreamHarness {
	t.Helper()
	installTinyGoTestHelpers(t)

	functionCtor := js.Global().Get("Function")
	if functionCtor.IsUndefined() || functionCtor.IsNull() {
		t.Skip("JavaScript Function constructor unavailable")
	}
	ready := make(chan struct{}, 1)
	delivered := make(chan struct{}, 8)
	pushed := make(chan struct{}, 8)
	ended := make(chan struct{}, 2)
	readyFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		signal(ready)
		return nil
	})
	pushedFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		signal(pushed)
		return nil
	})
	endedFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		signal(ended)
		return nil
	})
	deliveredFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		signal(delivered)
		return nil
	})
	t.Cleanup(func() {
		readyFn.Release()
		pushedFn.Release()
		endedFn.Release()
		deliveredFn.Release()
	})

	value := functionCtor.New(`
	const ready = arguments[0];
	const pushed = arguments[1];
	const ended = arguments[2];
	const delivered = arguments[3];
	const harness = {
		onMessage: undefined,
		onClose: undefined,
		resolve: undefined,
		reject: undefined,
		pushable: {
			messages: [],
			endCalls: 0,
		},
	};
	harness.openStream = (onMessage, onClose, onResolve, onReject) => {
		harness.onMessage = onMessage;
		harness.onClose = onClose;
		harness.resolve = onResolve;
		harness.reject = onReject;
		queueMicrotask(ready);
	};
	harness.pushable.push = (message) => {
		harness.pushable.messages.push(message);
		queueMicrotask(pushed);
	};
	harness.pushable.end = () => {
		harness.pushable.endCalls++;
		queueMicrotask(ended);
	};
	harness.resolveOpen = () => {
		harness.resolve(harness.pushable);
	};
	harness.rejectOpen = (message) => {
		harness.reject(message);
	};
	harness.deliver = (message) => {
		queueMicrotask(() => {
			harness.onMessage(message);
			delivered();
		});
	};
	return harness;
	`).Invoke(readyFn, pushedFn, endedFn, deliveredFn)
	return &openStreamHarness{
		value:     value,
		ready:     ready,
		delivered: delivered,
		pushed:    pushed,
		ended:     ended,
	}
}

func newAcceptStreamHarness(t *testing.T) *acceptStreamHarness {
	t.Helper()
	installTinyGoTestHelpers(t)

	functionCtor := js.Global().Get("Function")
	if functionCtor.IsUndefined() || functionCtor.IsNull() {
		t.Skip("JavaScript Function constructor unavailable")
	}
	callbackSet := make(chan struct{}, 1)
	started := make(chan struct{}, 1)
	posted := make(chan struct{}, 2)
	closed := make(chan struct{}, 1)
	callbackSetFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		signal(callbackSet)
		return nil
	})
	startedFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		signal(started)
		return nil
	})
	postedFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		signal(posted)
		return nil
	})
	closedFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		signal(closed)
		return nil
	})
	t.Cleanup(func() {
		callbackSetFn.Release()
		startedFn.Release()
		postedFn.Release()
		closedFn.Release()
	})

	value := functionCtor.New(`
	const callbackSet = arguments[0];
	const started = arguments[1];
	const posted = arguments[2];
	const closed = arguments[3];
	const harness = {
		callback: undefined,
		port: {
			postCalls: [],
			closeCalls: 0,
			startCalls: 0,
			postMessage(message) {
				this.postCalls.push(message);
				queueMicrotask(posted);
			},
			close() {
				this.closeCalls++;
				queueMicrotask(closed);
			},
			start() {
				this.startCalls++;
				queueMicrotask(started);
			},
		},
	};
	harness.setAcceptStream = (callback) => {
		harness.callback = callback;
		if (typeof callback === 'function') {
			queueMicrotask(callbackSet);
		}
	};
	harness.accept = () => {
		harness.callback(harness.port);
	};
	return harness;
	`).Invoke(callbackSetFn, startedFn, postedFn, closedFn)
	return &acceptStreamHarness{
		value:       value,
		callbackSet: callbackSet,
		started:     started,
		posted:      posted,
		closed:      closed,
	}
}
