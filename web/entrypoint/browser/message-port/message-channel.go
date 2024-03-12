//go:build js
// +build js

package message_port

import "syscall/js"

// NewMessageChannel constructs two connected MessagePort.
func NewMessageChannel() (port1, port2 js.Value) {
	global := js.Global()
	messageChannelCtor := global.Get("MessageChannel")
	messageChannel := messageChannelCtor.New()
	return messageChannel.Get("port1"), messageChannel.Get("port2")
}

// This works as expected:
/*
global := js.Global()
messageChannelCtor := global.Get("MessageChannel")
messageChannel := messageChannelCtor.New()
ports := []interface{}{messageChannel.Get("port1"), messageChannel.Get("port2")}
js.Global().Set("arrayOfPorts", ports)
*/
