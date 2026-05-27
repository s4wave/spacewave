//go:build js && !tinygo

package blockshard

import "syscall/js"

func useTinyGoBroadcastHelpers() bool {
	return false
}

func newTinyGoBroadcastChannel(_ string) js.Value {
	return js.Undefined()
}

func sendTinyGoBroadcast(_ js.Value, _ int, _ uint64) {}

func closeTinyGoBroadcastChannel(_ js.Value) {}
