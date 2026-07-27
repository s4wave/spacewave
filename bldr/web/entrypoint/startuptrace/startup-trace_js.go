//go:build js && bldr_startup_trace

// Package startuptrace installs the opt-in browser runtime startup trace.
package startuptrace

import (
	"bytes"
	"encoding/base64"
	"runtime/trace"
	"sync"
	"syscall/js"
)

type startupTrace struct {
	mu       sync.Mutex
	buffer   bytes.Buffer
	encoded  string
	stopped  bool
	cleaned  bool
	stopFunc js.Func
	readFunc js.Func
}

// Install starts the runtime trace and exposes chunked stop and read callbacks.
func Install() {
	owner := &startupTrace{}
	if err := trace.Start(&owner.buffer); err != nil {
		owner.buffer.Reset()
		return
	}
	owner.stopFunc = js.FuncOf(owner.stop)
	owner.readFunc = js.FuncOf(owner.read)
	js.Global().Set("BLDR_STOP_STARTUP_TRACE", owner.stopFunc)
	js.Global().Set("BLDR_READ_STARTUP_TRACE", owner.readFunc)
}

func (t *startupTrace) stop(_ js.Value, _ []js.Value) any {
	t.mu.Lock()
	if t.cleaned {
		t.mu.Unlock()
		return "error:startup trace unavailable"
	}
	if !t.stopped {
		trace.Stop()
		t.stopped = true
		t.encoded = base64.StdEncoding.EncodeToString(t.buffer.Bytes())
	}
	if t.encoded == "" {
		t.mu.Unlock()
		t.cleanup()
		return "error:startup trace is empty"
	}
	length := len(t.encoded)
	t.mu.Unlock()
	return length
}

func (t *startupTrace) read(_ js.Value, args []js.Value) any {
	if len(args) != 2 {
		t.cleanup()
		return ""
	}

	offset, size := args[0].Int(), args[1].Int()
	t.mu.Lock()
	if t.cleaned || !t.stopped || offset < 0 || size <= 0 || offset >= len(t.encoded) || size > len(t.encoded)-offset {
		t.mu.Unlock()
		t.cleanup()
		return ""
	}
	end := offset + size
	chunk := t.encoded[offset:end]
	final := end == len(t.encoded)
	t.mu.Unlock()
	if final {
		t.cleanup()
	}
	return chunk
}

func (t *startupTrace) cleanup() {
	t.mu.Lock()
	if t.cleaned {
		t.mu.Unlock()
		return
	}
	if !t.stopped {
		trace.Stop()
		t.stopped = true
	}
	t.cleaned = true
	stopFunc := t.stopFunc
	readFunc := t.readFunc
	t.stopFunc = js.Func{}
	t.readFunc = js.Func{}
	t.buffer.Reset()
	t.encoded = ""
	t.mu.Unlock()

	js.Global().Delete("BLDR_STOP_STARTUP_TRACE")
	js.Global().Delete("BLDR_READ_STARTUP_TRACE")
	stopFunc.Release()
	readFunc.Release()
}
