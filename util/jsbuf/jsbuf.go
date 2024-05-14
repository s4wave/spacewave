//go:build js

package jsbuf

import (
	"errors"
	"syscall/js"

	"github.com/hack-pad/safejs"
)

// uint8Array is the global reference to Uint8Array.
var uint8Array = safejs.Safe(js.Global().Get("Uint8Array"))

// arrayBuffer is the global reference to ArrayBuffer.
var arrayBuffer = safejs.Safe(js.Global().Get("ArrayBuffer"))

// ErrNotUint8Array is returned if the value is not a Uint8Array.
var ErrNotUint8Array = errors.New("value is not a Uint8Array")

// ErrIncorrectCopyLength is returned if the copied data length is incorrect.
var ErrIncorrectCopyLength = errors.New("copied incorrect data length to js")

// Uint8Array returns the global Uint8Array type.
func Uint8Array() safejs.Value {
	return uint8Array
}

// CheckIsUint8Array checks if the js value is a Uint8Array.
func CheckIsUint8Array(value safejs.Value) bool {
	isArr, err := value.InstanceOf(Uint8Array())
	if err != nil {
		return false
	}
	return isArr
}

// CastToUint8Array attempts to cast the value to a Uint8Array.
func CastToUint8Array(value safejs.Value) (safejs.Value, error) {
	if CheckIsUint8Array(value) {
		return value, nil
	}
	if CheckIsArrayBuffer(value) {
		return Uint8Array().New(value)
	}
	return value, ErrNotUint8Array
}

// CopyBytesToGo converts a js.Value Uint8Array to a []byte.
//
// Returns an error if value is not a Uint8Array.
func CopyBytesToGo(value safejs.Value) ([]byte, error) {
	arr, err := CastToUint8Array(value)
	if err != nil {
		return nil, err
	}
	len, err := arr.Length()
	if err != nil {
		return nil, err
	}
	buf := make([]byte, len)
	n, err := safejs.CopyBytesToGo(buf, arr)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

// CopyBytesToJs converts a []byte to a js.Value Uint8Array.
func CopyBytesToJs(key []byte) (safejs.Value, error) {
	// convert []byte to js.Value
	keyArr, err := Uint8Array().New(len(key))
	if err != nil {
		return safejs.Null(), err
	}
	n, err := safejs.CopyBytesToJS(keyArr, key)
	if err != nil {
		return safejs.Null(), err
	}
	if n != len(key) {
		return safejs.Null(), ErrIncorrectCopyLength
	}
	return keyArr, nil
}

// ArrayBuffer returns the global ArrayBuffer type.
func ArrayBuffer() safejs.Value {
	return arrayBuffer
}

// CheckIsArrayBuffer checks if the js value is an ArrayBuffer.
func CheckIsArrayBuffer(value safejs.Value) bool {
	isArr, err := value.InstanceOf(ArrayBuffer())
	if err != nil {
		return false
	}
	return isArr
}
