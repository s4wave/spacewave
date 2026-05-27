//go:build js && tinygo

package blockshard

import (
	"syscall/js"
	"unsafe"
)

//go:wasmimport gojs bldr.opfs.broadcastChannelNewRef
func tinyGoBroadcastChannelNewRef(namePtr unsafe.Pointer, nameLen uint32) uint64

//go:wasmimport gojs bldr.opfs.broadcastSendRef
func tinyGoBroadcastSendRef(channelRef uint64, shardID uint32, generationHi uint32, generationLo uint32)

//go:wasmimport gojs bldr.opfs.broadcastCloseRef
func tinyGoBroadcastCloseRef(channelRef uint64)

type tinyGoJSValue struct {
	_     [0]func()
	ref   uint64
	gcPtr unsafe.Pointer
}

func useTinyGoBroadcastHelpers() bool {
	return true
}

func tinyGoJSRef(value js.Value) uint64 {
	return (*tinyGoJSValue)(unsafe.Pointer(&value)).ref
}

func tinyGoJSValueFromRef(ref uint64) js.Value {
	value := tinyGoJSValue{ref: ref}
	return *(*js.Value)(unsafe.Pointer(&value))
}

func newTinyGoBroadcastChannel(name string) js.Value {
	nameBytes := []byte(name)
	if len(nameBytes) == 0 {
		return tinyGoJSValueFromRef(tinyGoBroadcastChannelNewRef(nil, 0))
	}
	return tinyGoJSValueFromRef(tinyGoBroadcastChannelNewRef(unsafe.Pointer(&nameBytes[0]), uint32(len(nameBytes))))
}

func sendTinyGoBroadcast(channel js.Value, shardID int, generation uint64) {
	tinyGoBroadcastSendRef(
		tinyGoJSRef(channel),
		uint32(shardID),
		uint32(generation>>32),
		uint32(generation),
	)
}

func closeTinyGoBroadcastChannel(channel js.Value) {
	tinyGoBroadcastCloseRef(tinyGoJSRef(channel))
}
