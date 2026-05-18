//go:build js

package web_runtime_wasm

import (
	"context"
	"errors"
	"strings"
	"sync"
	"syscall/js"
	"testing"
	"time"
)

func TestNewPushableOpenStreamRetainsPromiseCallbacksUntilSettlement(t *testing.T) {
	console := captureConsoleError(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	started := make(chan struct{}, 1)
	var resolve js.Value
	var onClose js.Value
	openFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		onClose = args[1]
		executor := js.FuncOf(func(this js.Value, args []js.Value) any {
			resolve = args[0]
			return nil
		})
		promise := js.Global().Get("Promise").New(executor)
		executor.Release()
		started <- struct{}{}
		return promise
	})
	defer openFn.Release()

	errCh := make(chan error, 1)
	go func() {
		_, err := NewPushableOpenStream(openFn.Value)(
			ctx,
			func([]byte) error { return nil },
			func(error) {},
		)
		errCh <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("open stream function was not invoked")
	}

	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("open stream did not return after context cancellation")
	}

	pushable, releasePushable := newTestJSPushable(t)
	defer releasePushable()
	resolve.Invoke(pushable)
	waitForJSTick(t)
	if !pushable.Get("__ended").Bool() {
		t.Fatal("expected canceled open stream to end the late pushable")
	}

	onClose.Invoke()
	waitForJSTick(t)
	console.assertNoReleasedFunction(t)
}

func TestNewPushableOpenStreamRetainsPacketCallbacksUntilOnClose(t *testing.T) {
	console := captureConsoleError(t)

	pushable, releasePushable := newTestJSPushable(t)
	defer releasePushable()

	started := make(chan struct{}, 1)
	var onClose js.Value
	openFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		onClose = args[1]
		started <- struct{}{}
		return js.Global().Get("Promise").Call("resolve", pushable)
	})
	defer openFn.Release()

	writerCh := make(chan interface {
		Close() error
	}, 1)
	errCh := make(chan error, 1)
	go func() {
		writer, err := NewPushableOpenStream(openFn.Value)(
			context.Background(),
			func([]byte) error { return nil },
			func(error) {},
		)
		if err != nil {
			errCh <- err
			return
		}
		writerCh <- writer
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("open stream function was not invoked")
	}
	waitForJSTick(t)

	var writer interface {
		Close() error
	}
	select {
	case writer = <-writerCh:
	case err := <-errCh:
		t.Fatalf("open stream failed: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("open stream did not resolve")
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	if !pushable.Get("__ended").Bool() {
		t.Fatal("expected writer close to end the pushable")
	}

	onClose.Invoke()
	waitForJSTick(t)
	console.assertNoReleasedFunction(t)
}

type consoleErrorCapture struct {
	mu       sync.Mutex
	original js.Value
	fn       js.Func
	messages []string
}

func captureConsoleError(t *testing.T) *consoleErrorCapture {
	t.Helper()

	console := js.Global().Get("console")
	capture := &consoleErrorCapture{
		original: console.Get("error"),
	}
	capture.fn = js.FuncOf(func(this js.Value, args []js.Value) any {
		capture.mu.Lock()
		defer capture.mu.Unlock()
		parts := make([]string, 0, len(args))
		for _, arg := range args {
			parts = append(parts, arg.String())
		}
		capture.messages = append(capture.messages, strings.Join(parts, " "))
		return nil
	})
	console.Set("error", capture.fn)
	t.Cleanup(func() {
		console.Set("error", capture.original)
		capture.fn.Release()
	})
	return capture
}

func (c *consoleErrorCapture) assertNoReleasedFunction(t *testing.T) {
	t.Helper()

	c.mu.Lock()
	defer c.mu.Unlock()
	for _, msg := range c.messages {
		if strings.Contains(msg, "call to released function") {
			t.Fatalf("unexpected released js.Func call: %s", msg)
		}
	}
}

func newTestJSPushable(t *testing.T) (js.Value, func()) {
	t.Helper()

	pushable := js.Global().Get("Object").New()
	pushable.Set("__ended", false)
	pushFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		return nil
	})
	endFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		pushable.Set("__ended", true)
		return nil
	})
	pushable.Set("push", pushFn)
	pushable.Set("end", endFn)

	return pushable, func() {
		pushFn.Release()
		endFn.Release()
	}
}

func waitForJSTick(t *testing.T) {
	t.Helper()

	done := make(chan struct{}, 1)
	fn := js.FuncOf(func(this js.Value, args []js.Value) any {
		done <- struct{}{}
		return nil
	})
	defer fn.Release()
	js.Global().Get("Promise").Call("resolve").Call("then", fn)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for JavaScript microtask")
	}
}
