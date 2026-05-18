//go:build js

package message_port

import (
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
