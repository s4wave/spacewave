//go:build js

package jsbuf

import (
	"errors"
	"syscall/js"
	"testing"

	"github.com/hack-pad/safejs"
)

func TestUint8Array(t *testing.T) {
	arr := Uint8Array()
	if arr.IsNull() {
		t.Error("Uint8Array() returned null")
	}
}

func TestCastToUint8Array(t *testing.T) {
	// Valid Uint8Array
	arr, _ := Uint8Array().New(0)
	casted, err := CastToUint8Array(arr)
	if err != nil {
		t.Errorf("CastToUint8Array() error = %v", err)
	}
	if !CheckIsUint8Array(casted) {
		t.Error("CastToUint8Array() did not return a Uint8Array")
	}

	// Valid ArrayBuffer
	buf, _ := arrayBuffer.New(0)
	casted, err = CastToUint8Array(buf)
	if err != nil {
		t.Errorf("CastToUint8Array() error = %v", err)
	}
	if !CheckIsUint8Array(casted) {
		t.Error("CastToUint8Array() did not return a Uint8Array")
	}

	// Invalid type
	notArr := safejs.Safe(js.Global().Get("Array").New(0))
	_, err = CastToUint8Array(notArr)
	if !errors.Is(err, ErrNotUint8Array) {
		t.Errorf("CastToUint8Array() error = %v, want %v", err, ErrNotUint8Array)
	}
}

func TestCopyBytesToGo(t *testing.T) {
	// Valid Uint8Array
	data := []byte{1, 2, 3}
	arr, err := CopyBytesToJs(data)
	if err != nil {
		t.Errorf("new Uint8Array error = %v", err)
	}
	got, err := CopyBytesToGo(arr)
	if err != nil {
		t.Errorf("CopyBytesToGo() error = %v", err)
	}
	if len(got) != len(data) {
		t.Errorf("CopyBytesToGo() len = %v, want %v", len(got), len(data))
	}
	for i, b := range got {
		if b != data[i] {
			t.Errorf("CopyBytesToGo() byte %d = %v, want %v", i, b, data[i])
		}
	}

	// Valid ArrayBuffer
	ubuf, err := CopyBytesToJs(data)
	if err != nil {
		t.Errorf("new Uint8Array error = %v", err)
	}
	buf, err := ubuf.Get("buffer")
	if err != nil {
		t.Errorf("new ArrayBuffer(arr) error = %v", err)
	}

	got, err = CopyBytesToGo(buf)
	if err != nil {
		t.Errorf("CopyBytesToGo() error = %v", err)
	}
	if len(got) != len(data) {
		t.Errorf("CopyBytesToGo() len = %v, want %v", len(got), len(data))
	}
	for i, b := range got {
		if b != data[i] {
			t.Errorf("CopyBytesToGo() byte %d = %v, want %v", i, b, data[i])
		}
	}

	// Invalid type
	notArr := safejs.Safe(js.Global().Get("Array").New(0))
	_, err = CopyBytesToGo(notArr)
	if !errors.Is(err, ErrNotUint8Array) {
		t.Errorf("CopyBytesToGo() error = %v, want %v", err, ErrNotUint8Array)
	}
}

func TestCopyBytesToJs(t *testing.T) {
	data := []byte{1, 2, 3}
	arr, err := CopyBytesToJs(data)
	if err != nil {
		t.Errorf("CopyBytesToJs() error = %v", err)
	}
	if !CheckIsUint8Array(arr) {
		t.Error("CopyBytesToJs() did not return a Uint8Array")
	}
	alen, _ := arr.Length()
	if alen != len(data) {
		t.Errorf("CopyBytesToJs() len = %v, want %v", alen, len(data))
	}
	for i := 0; i < alen; i++ {
		got := safejs.Unsafe(arr).Index(i).Int()
		if got != int(data[i]) {
			t.Errorf("CopyBytesToJs() byte %d = %v, want %v", i, got, data[i])
		}
	}
}
func TestCheckIsArrayBuffer(t *testing.T) {
	buf, _ := arrayBuffer.New(0)
	if !CheckIsArrayBuffer(buf) {
		t.Error("CheckIsArrayBuffer() returned false for ArrayBuffer")
	}

	notBuf := safejs.Safe(js.Global().Get("Array").New(0))
	if CheckIsArrayBuffer(notBuf) {
		t.Error("CheckIsArrayBuffer() returned true for non-ArrayBuffer")
	}
}
