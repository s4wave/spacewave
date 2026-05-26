//go:build js && tinygo

package jsbuf

import "unsafe"

var (
	tinyGoBytesNextID uint32 = 1
	tinyGoBytes              = make(map[uint32][]byte)
)

// WithTinyGoBytes exposes bytes to JavaScript by a small ID for the duration
// of call. JavaScript must copy the bytes before returning from the helper.
func WithTinyGoBytes(bytes []byte, call func(id uint32)) {
	id, release := HoldTinyGoBytes(bytes)
	defer release()
	call(id)
}

// HoldTinyGoBytes exposes bytes to JavaScript until the returned release
// function is called.
func HoldTinyGoBytes(bytes []byte) (uint32, func()) {
	id := tinyGoBytesNextID
	tinyGoBytesNextID++
	if tinyGoBytesNextID == 0 {
		tinyGoBytesNextID = 1
	}

	tinyGoBytes[id] = bytes
	return id, func() {
		delete(tinyGoBytes, id)
	}
}

//go:wasmexport BLDR_TINYGO_BYTES_PTR
func tinyGoBytesPtr(id uint32) uint32 {
	bytes := tinyGoBytes[id]
	if len(bytes) == 0 {
		return 0
	}
	return uint32(uintptr(unsafe.Pointer(&bytes[0])))
}

//go:wasmexport BLDR_TINYGO_BYTES_LEN
func tinyGoBytesLen(id uint32) uint32 {
	return uint32(len(tinyGoBytes[id]))
}
