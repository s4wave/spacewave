//go:build js

package web_runtime_wasm

import (
	"bytes"
	"context"
	"io"
	"runtime"
	"syscall/js"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
)

// openStreamHarness wraps the JS open-stream test harness object.
type openStreamHarness struct {
	// value is the JS harness object.
	value js.Value

	// ready signals the open stream call arrived.
	ready chan struct{}
	// delivered signals a message was delivered to the callbacks.
	delivered chan struct{}
	// pushed signals a packet was pushed into the sink.
	pushed chan struct{}
	// ended signals the sink ended.
	ended chan struct{}
}

// Get returns a property of the JS harness.
func (h *openStreamHarness) Get(name string) js.Value {
	return h.value.Get(name)
}

// Call invokes a JS harness operation.
func (h *openStreamHarness) Call(name string, args ...any) js.Value {
	return h.value.Call(name, args...)
}

// acceptStreamHarness wraps the JS accept-stream test harness object.
type acceptStreamHarness struct {
	// value is the JS harness object.
	value js.Value

	// callbackSet signals the accept callback was registered.
	callbackSet chan struct{}
	// started signals the accepted port started.
	started chan struct{}
	// posted signals a postMessage call on the port.
	posted chan struct{}
	// closed signals the port closed.
	closed chan struct{}
}

// Get returns a property of the JS harness.
func (h *acceptStreamHarness) Get(name string) js.Value {
	return h.value.Get(name)
}

// Call invokes a JS harness operation.
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

	pkt := srpc.NewCallStartPacket("service", "method", []byte{0, 127, 128, 255}, false)
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
			// Pass data without cloning: the delivery owner must supply the copy.
			started <- data
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

	// Boundary byte values must survive the copy from the Uint8Array.
	src := jsUint8Array([]byte{0, 127, 128, 255})
	harness.Call("deliver", src)
	boundary := waitForMessage(t, started, "boundary message")
	if !bytes.Equal(boundary, []byte{0, 127, 128, 255}) {
		t.Fatalf("boundary message = %v want [0 127 128 255]", boundary)
	}
	waitForSignal(t, harness.delivered, "boundary delivered callback")

	// The handler must own a copy: mutating the source array after delivery
	// cannot change the bytes the handler already received.
	src.SetIndex(0, 255)
	if !bytes.Equal(boundary, []byte{0, 127, 128, 255}) {
		t.Fatalf("handler observed source mutation: %v", boundary)
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

// copyJSBytes reads the JS bytes independently of the production bulk copier.
func copyJSBytes(value js.Value) []byte {
	data := make([]byte, value.Length())
	for i := range data {
		data[i] = byte(value.Index(i).Int())
	}
	return data
}

// startOpenStreamHarness opens a stream and waits for the JS harness to receive it.
func startOpenStreamHarness(
	t *testing.T,
	harness *openStreamHarness,
	msgHandler srpc.PacketDataHandler,
	closeHandler srpc.CloseHandler,
) srpc.PacketWriter {
	t.Helper()
	packetCallbacks := &jsStreamCallbacks{}
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
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the open stream harness")
	}
	return writer
}

// jsUint8Array constructs test input independently of the production copier.
func jsUint8Array(data []byte) js.Value {
	arr := js.Global().Get("Uint8Array").New(len(data))
	for i, b := range data {
		arr.SetIndex(i, int(b))
	}
	return arr
}

// waitForMessage waits for a delivered message or fails after 2 seconds.
func waitForMessage(t *testing.T, ch <-chan []byte, name string) []byte {
	t.Helper()
	runtime.Gosched()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
		return nil
	}
}

// receiveMessage reads an already available message without waiting.
func receiveMessage(ch <-chan []byte) ([]byte, bool) {
	select {
	case msg := <-ch:
		return msg, true
	default:
		return nil, false
	}
}

// receiveError reads an already available error without waiting.
func receiveError(ch <-chan error) (error, bool) {
	select {
	case err := <-ch:
		return err, true
	default:
		return nil, false
	}
}

// waitForSignal waits for a signal or fails after 2 seconds.
func waitForSignal(t *testing.T, ch <-chan struct{}, name string) {
	t.Helper()
	runtime.Gosched()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

// waitForError waits for an error or fails after 2 seconds.
func waitForError(t *testing.T, ch <-chan error, name string) error {
	t.Helper()
	runtime.Gosched()
	select {
	case err := <-ch:
		return err
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
		return nil
	}
}

// signal records a pending notification without blocking the JS callback.
func signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// installTinyGoTestHelpers installs byte bridge helpers for the test lifetime.
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

// copyUint8Array clones the input with JS-owned storage.
func copyUint8Array(value js.Value) js.Value {
	cloned := js.Global().Get("Uint8Array").New(value.Get("byteLength").Int())
	cloned.Call("set", value)
	return cloned
}

// newOpenStreamHarness constructs a controllable JS stream bridge.
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

// newAcceptStreamHarness constructs a JS bridge with an observable MessagePort.
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
