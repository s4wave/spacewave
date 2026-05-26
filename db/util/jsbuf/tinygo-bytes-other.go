//go:build js && !tinygo

package jsbuf

import "syscall/js"

// NewBytes constructs a JavaScript-owned Uint8Array.
func NewBytes(size int) (js.Value, error) {
	return js.Global().Get("Uint8Array").New(size), nil
}

// CopyBytesToJS copies Go bytes into a JavaScript-owned Uint8Array.
func CopyBytesToJS(bytes []byte) (js.Value, error) {
	arr, err := NewBytes(len(bytes))
	if err != nil {
		return js.Value{}, err
	}
	if len(bytes) != 0 {
		js.CopyBytesToJS(arr, bytes)
	}
	return arr, nil
}

// CopyStoredBytes is unavailable outside TinyGo helper mode.
func CopyStoredBytes(_ int, _ []byte) (int, bool) {
	return 0, false
}
