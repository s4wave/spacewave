//go:build js

package message_port

import (
	"bytes"
	"context"
	"io"
	"runtime"
	"syscall/js"
	"testing"
)

func TestMessagePortCloseClearsHandlerAndWakesRead(t *testing.T) {
	chObj := newTestMessagePort(t)
	port := NewMessagePort(chObj)
	if typ := chObj.Get("onmessage").Type(); typ != js.TypeFunction {
		t.Fatalf("expected onmessage function, got %s", typ.String())
	}
	if calls := chObj.Get("startCalls").Int(); calls != 1 {
		t.Fatalf("expected start to be called once, got %d", calls)
	}

	readDone := make(chan error, 1)
	go func() {
		_, err := port.ReadMessage(context.Background())
		readDone <- err
	}()

	for i := 0; i < 100 && port.trig == nil; i++ {
		runtime.Gosched()
	}
	if port.trig == nil {
		t.Fatal("read did not block on port trigger")
	}

	if err := port.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	err := waitReadError(readDone)
	if err != io.EOF {
		t.Fatalf("expected blocked read to wake with EOF, got %v", err)
	}

	if !chObj.Get("onmessage").IsNull() {
		t.Fatal("expected close to clear onmessage")
	}
	if msgCount := chObj.Get("messages").Length(); msgCount != 1 {
		t.Fatalf("expected one close message, got %d", msgCount)
	}
	if !chObj.Get("messages").Index(0).IsNull() {
		t.Fatal("expected close message to be null")
	}
	if calls := chObj.Get("closeCalls").Int(); calls != 1 {
		t.Fatalf("expected JS close to be called once, got %d", calls)
	}

	if err := port.Close(); err != nil {
		t.Fatalf("second close failed: %v", err)
	}
	if msgCount := chObj.Get("messages").Length(); msgCount != 1 {
		t.Fatalf("expected second close not to post again, got %d messages", msgCount)
	}
	if calls := chObj.Get("closeCalls").Int(); calls != 1 {
		t.Fatalf("expected second close not to call JS close again, got %d", calls)
	}
}

func TestMessagePortReadPreservesPostMessageOrder(t *testing.T) {
	chObj := newTestMessagePort(t)
	port := NewMessagePort(chObj)

	deliverTestMessage(t, chObj, []byte("first"))
	deliverTestMessage(t, chObj, []byte("second"))
	deliverTestMessage(t, chObj, []byte("third"))

	for _, want := range []string{"first", "second", "third"} {
		got, err := port.ReadMessage(context.Background())
		if err != nil {
			t.Fatalf("read message: %v", err)
		}
		if string(got) != want {
			t.Fatalf("message order mismatch: got %q want %q", string(got), want)
		}
	}
}

func TestMessagePortReadDrainsQueuedMessagesBeforeClose(t *testing.T) {
	chObj := newTestMessagePort(t)
	port := NewMessagePort(chObj)

	deliverTestMessage(t, chObj, []byte("first"))
	deliverTestMessage(t, chObj, []byte("second"))
	deliverTestClose(t, chObj)

	for _, want := range []string{"first", "second"} {
		got, err := port.ReadMessage(context.Background())
		if err != nil {
			t.Fatalf("read message: %v", err)
		}
		if string(got) != want {
			t.Fatalf("message order mismatch: got %q want %q", string(got), want)
		}
	}

	if _, err := port.ReadMessage(context.Background()); err != io.EOF {
		t.Fatalf("expected EOF after queued messages, got %v", err)
	}
}

func TestMessagePortReadDrainsBodyChunkAndTailBeforeClose(t *testing.T) {
	chObj := newTestMessagePort(t)
	port := NewMessagePort(chObj)

	bodyChunk := make([]byte, 32*1024)
	for i := range bodyChunk {
		bodyChunk[i] = byte(i % 251)
	}
	tail := []byte("export{App as default};\n")
	deliverTestMessage(t, chObj, bodyChunk)
	deliverTestMessage(t, chObj, tail)
	deliverTestClose(t, chObj)

	gotChunk, err := port.ReadMessage(context.Background())
	if err != nil {
		t.Fatalf("read body chunk: %v", err)
	}
	if !bytes.Equal(gotChunk, bodyChunk) {
		t.Fatal("body chunk changed while queued before close")
	}

	gotTail, err := port.ReadMessage(context.Background())
	if err != nil {
		t.Fatalf("read tail: %v", err)
	}
	if string(gotTail) != string(tail) {
		t.Fatalf("tail mismatch: got %q want %q", string(gotTail), string(tail))
	}

	if _, err := port.ReadMessage(context.Background()); err != io.EOF {
		t.Fatalf("expected EOF after body chunk and tail, got %v", err)
	}
}

func TestMessagePortPacketStreamClosePreservesInboundRead(t *testing.T) {
	chObj := newTestMessagePort(t)
	port := NewMessagePort(chObj)
	stream := NewMessagePortPacketStream(port)

	if err := stream.Close(); err != nil {
		t.Fatalf("packet stream close failed: %v", err)
	}
	if msgCount := chObj.Get("messages").Length(); msgCount != 1 {
		t.Fatalf("expected one outbound close message, got %d", msgCount)
	}
	if !chObj.Get("messages").Index(0).IsNull() {
		t.Fatal("expected outbound close message to be null")
	}
	if calls := chObj.Get("closeCalls").Int(); calls != 0 {
		t.Fatalf("packet stream close should not close JS port, got %d calls", calls)
	}
	if typ := chObj.Get("onmessage").Type(); typ != js.TypeFunction {
		t.Fatalf("packet stream close should preserve inbound handler, got %s", typ.String())
	}

	deliverTestMessage(t, chObj, []byte("response-info"))
	deliverTestMessage(t, chObj, []byte("response-body"))
	deliverTestClose(t, chObj)

	for _, want := range []string{"response-info", "response-body"} {
		got, err := port.ReadMessage(context.Background())
		if err != nil {
			t.Fatalf("read message: %v", err)
		}
		if string(got) != want {
			t.Fatalf("message order mismatch: got %q want %q", string(got), want)
		}
	}

	if _, err := port.ReadMessage(context.Background()); err != io.EOF {
		t.Fatalf("expected EOF after remote close, got %v", err)
	}
	if err := port.Close(); err != nil {
		t.Fatalf("full close failed: %v", err)
	}
	if msgCount := chObj.Get("messages").Length(); msgCount != 1 {
		t.Fatalf("expected full close not to post a second outbound close, got %d messages", msgCount)
	}
}

func waitReadError(readDone <-chan error) error {
	for range 100 {
		select {
		case err := <-readDone:
			return err
		default:
			runtime.Gosched()
		}
	}
	return context.DeadlineExceeded
}

func deliverTestMessage(t *testing.T, chObj js.Value, data []byte) {
	t.Helper()

	msg := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(msg, data)
	event := js.Global().Get("Object").New()
	event.Set("data", msg)
	chObj.Get("onmessage").Invoke(event)
}

func deliverTestClose(t *testing.T, chObj js.Value) {
	t.Helper()

	event := js.Global().Get("Object").New()
	event.Set("data", js.Null())
	chObj.Get("onmessage").Invoke(event)
}

func newTestMessagePort(t *testing.T) js.Value {
	t.Helper()

	functionCtor := js.Global().Get("Function")
	if functionCtor.IsUndefined() || functionCtor.IsNull() {
		t.Skip("JavaScript Function constructor unavailable")
	}

	return functionCtor.New(`
return {
	messages: [],
	closeCalls: 0,
	startCalls: 0,
	onmessage: undefined,
	postMessage: function(message) {
		this.messages.push(message);
	},
	close: function() {
		this.closeCalls++;
	},
	start: function() {
		this.startCalls++;
	}
};
`).Invoke()
}
