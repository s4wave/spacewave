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

func TestNewPushableOpenStreamBuffersBeforeJSStreamResolves(t *testing.T) {
	harness := newOpenStreamHarness(t)
	writer, err := NewPushableOpenStream(harness.Get("openStream"))(
		context.Background(),
		func(data []byte) error { return nil },
		func(closeErr error) {},
	)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}

	pkt := srpc.NewCallStartPacket("service", "method", []byte("first"), false)
	if err := writer.WritePacket(pkt); err != nil {
		t.Fatalf("write queued packet: %v", err)
	}
	if got := harness.Get("pushable").Get("messages").Length(); got != 0 {
		t.Fatalf("packet pushed before stream resolved: %d", got)
	}

	waitOpenStreamHarnessReady(t, harness)
	harness.Call("resolveOpen")
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
}

func TestNewPushableOpenStreamCloseBeforeJSStreamResolves(t *testing.T) {
	harness := newOpenStreamHarness(t)
	writer, err := NewPushableOpenStream(harness.Get("openStream"))(
		context.Background(),
		func(data []byte) error { return nil },
		func(closeErr error) {},
	)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}

	waitOpenStreamHarnessReady(t, harness)
	if err := writer.Close(); err != nil {
		t.Fatalf("close pending writer: %v", err)
	}
	harness.Call("resolveOpen")
	if got := harness.Get("pushable").Get("endCalls").Int(); got != 1 {
		t.Fatalf("pushable end calls = %d want 1", got)
	}
	if err := writer.WritePacket(srpc.NewCallCancelPacket()); err != io.ErrClosedPipe {
		t.Fatalf("write after close = %v want %v", err, io.ErrClosedPipe)
	}
}

func TestNewPushableOpenStreamRejectFailsPendingWriter(t *testing.T) {
	harness := newOpenStreamHarness(t)
	closeErrCh := make(chan error, 1)
	writer, err := NewPushableOpenStream(harness.Get("openStream"))(
		context.Background(),
		func(data []byte) error { return nil },
		func(closeErr error) { closeErrCh <- closeErr },
	)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	if err := writer.WritePacket(srpc.NewCallCancelPacket()); err != nil {
		t.Fatalf("write queued packet: %v", err)
	}

	waitOpenStreamHarnessReady(t, harness)
	harness.Call("rejectOpen", "boom")
	closeErr := <-closeErrCh
	if closeErr == nil || closeErr.Error() != "boom" {
		t.Fatalf("close error = %v want boom", closeErr)
	}
	if err := writer.WritePacket(srpc.NewCallCancelPacket()); err == nil || err.Error() != "boom" {
		t.Fatalf("write after reject = %v want boom", err)
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

	waitForJSFunc(t, harness.Get("callback"), "accept stream callback")
	harness.Call("accept")
	waitForJSInt(t, harness.Get("port"), "startCalls", 1)
	if got := harness.Get("port").Get("postCalls").Length(); got != 0 {
		t.Fatalf("post calls before close = %d want 0", got)
	}

	cancel()
	waitForJSInt(t, harness.Get("port"), "closeCalls", 1)
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

func waitOpenStreamHarnessReady(t *testing.T, harness js.Value) {
	t.Helper()
	for range 100 {
		if harness.Get("resolve").Type() == js.TypeFunction && harness.Get("reject").Type() == js.TypeFunction {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("open stream harness was not initialized")
}

func waitForJSFunc(t *testing.T, value js.Value, name string) {
	t.Helper()
	for range 100 {
		if value.Type() == js.TypeFunction {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("%s was not initialized", name)
}

func waitForJSInt(t *testing.T, obj js.Value, property string, want int) {
	t.Helper()
	for range 100 {
		if got := obj.Get(property).Int(); got == want {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("%s = %d want %d", property, obj.Get(property).Int(), want)
}

func newOpenStreamHarness(t *testing.T) js.Value {
	t.Helper()

	functionCtor := js.Global().Get("Function")
	if functionCtor.IsUndefined() || functionCtor.IsNull() {
		t.Skip("JavaScript Function constructor unavailable")
	}

	return functionCtor.New(`
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
	};
	harness.pushable.push = (message) => {
		harness.pushable.messages.push(message);
	};
	harness.pushable.end = () => {
		harness.pushable.endCalls++;
	};
	harness.resolveOpen = () => {
		harness.resolve(harness.pushable);
	};
	harness.rejectOpen = (message) => {
		harness.reject(message);
	};
	return harness;
`).Invoke()
}

func newAcceptStreamHarness(t *testing.T) js.Value {
	t.Helper()

	functionCtor := js.Global().Get("Function")
	if functionCtor.IsUndefined() || functionCtor.IsNull() {
		t.Skip("JavaScript Function constructor unavailable")
	}

	return functionCtor.New(`
const harness = {
	callback: undefined,
	port: {
		postCalls: [],
		closeCalls: 0,
		startCalls: 0,
		postMessage(message) {
			this.postCalls.push(message);
		},
		close() {
			this.closeCalls++;
		},
		start() {
			this.startCalls++;
		},
	},
};
harness.setAcceptStream = (callback) => {
	harness.callback = callback;
};
harness.accept = () => {
	harness.callback(harness.port);
};
return harness;
`).Invoke()
}
