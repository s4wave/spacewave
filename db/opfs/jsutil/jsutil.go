//go:build js

// Package jsutil provides JavaScript call helpers for OPFS code.
package jsutil

import (
	"runtime"
	"syscall/js"

	"github.com/s4wave/spacewave/db/util/jsbuf"
)

const (
	tinyGoJSCall       = "BLDR_TINYGO_JS_CALL"
	tinyGoJSNew        = "BLDR_TINYGO_JS_NEW"
	tinyGoPromiseAwait = "BLDR_TINYGO_PROMISE_AWAIT"
)

// Call calls target[method](...args), using a JS-owned helper when available.
func Call(target js.Value, method string, args ...any) js.Value {
	if UseTinyGoHelpers() {
		call := js.Global().Get(tinyGoJSCall)
		if !Available(call) {
			return target.Call(method, args...)
		}
		helperArgs := make([]any, 0, len(args)+2)
		helperArgs = append(helperArgs, target, method)
		helperArgs = append(helperArgs, args...)
		return call.Invoke(helperArgs...)
	}
	return target.Call(method, args...)
}

// New constructs ctor with args, using a JS-owned helper when available.
func New(ctor js.Value, args ...any) js.Value {
	if UseTinyGoHelpers() {
		newValue := js.Global().Get(tinyGoJSNew)
		if !Available(newValue) {
			return ctor.New(args...)
		}
		helperArgs := make([]any, 0, len(args)+1)
		helperArgs = append(helperArgs, ctor)
		helperArgs = append(helperArgs, args...)
		return newValue.Invoke(helperArgs...)
	}
	return ctor.New(args...)
}

// NewObject constructs a JavaScript Object.
func NewObject() js.Value {
	return New(js.Global().Get("Object"))
}

// NewPromise constructs a JavaScript Promise.
func NewPromise(executor js.Func) js.Value {
	return New(js.Global().Get("Promise"), executor)
}

// NewUint8Array constructs a JavaScript Uint8Array.
func NewUint8Array(args ...any) js.Value {
	return New(js.Global().Get("Uint8Array"), args...)
}

// AwaitPromise attaches promise resolve/reject callbacks.
func AwaitPromise(promise js.Value, thenCb, catchCb js.Func) {
	if UseTinyGoHelpers() {
		await := js.Global().Get(tinyGoPromiseAwait)
		if !Available(await) {
			Call(promise, "then", thenCb, catchCb)
			return
		}
		await.Invoke(promise, thenCb, catchCb)
		return
	}
	Call(promise, "then", thenCb, catchCb)
}

// UseTinyGoHelpers reports whether TinyGo-specific JavaScript helpers may be used.
func UseTinyGoHelpers() bool {
	return runtime.Compiler == "tinygo"
}

// Available returns true if fn is a callable JavaScript value.
func Available(fn js.Value) bool {
	return !fn.IsUndefined() && !fn.IsNull() && fn.Type() == js.TypeFunction
}

// CopyStoredBytes copies a JS-owned stored byte slice into dst and consumes it.
func CopyStoredBytes(id int, dst []byte) (int, bool) {
	if !UseTinyGoHelpers() {
		return 0, false
	}
	return jsbuf.CopyStoredBytes(id, dst)
}
