//go:build js && tinygo

package jsbuf

import (
	"errors"
	"syscall/js"
)

const (
	tinyGoNewBytes        = "BLDR_TINYGO_NEW_BYTES"
	tinyGoTakeStoredBytes = "BLDR_TINYGO_TAKE_STORED_BYTES"
)

// NewBytes constructs a JavaScript-owned Uint8Array.
func NewBytes(size int) (js.Value, error) {
	newBytes := js.Global().Get(tinyGoNewBytes)
	if !available(newBytes) {
		return js.Value{}, errors.New("tinygo new bytes helper unavailable")
	}
	return newBytes.Invoke(size), nil
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

// CopyStoredBytes copies and consumes a JavaScript-owned stored byte slice.
func CopyStoredBytes(id int, dst []byte) (int, bool) {
	takeBytes := js.Global().Get(tinyGoTakeStoredBytes)
	if !available(takeBytes) {
		return 0, false
	}
	arr := takeBytes.Invoke(id)
	if arr.IsUndefined() || arr.IsNull() {
		return 0, true
	}
	return js.CopyBytesToGo(dst, arr), true
}

func available(fn js.Value) bool {
	return !fn.IsUndefined() && !fn.IsNull() && fn.Type() == js.TypeFunction
}
