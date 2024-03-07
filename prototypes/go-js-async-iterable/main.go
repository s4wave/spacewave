package main

import (
	"fmt"
	"syscall/js"
)

func main() {
	js.Global().Set("iteratePacketStream", js.FuncOf(iteratePacketStream))
	<-make(chan bool)
}

func iteratePacketStream(this js.Value, args []js.Value) interface{} {
	packetStream := args[0]

	iterator := packetStream.Call("getReader")
	promise := js.Global().Get("Promise").New(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		resolve, reject := args[0], args[1]

		var iteratePackets func()
		iteratePackets = func() {
			promise := iterator.Call("read")
			promise.Call("then", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				result := args[0]
				if result.Get("done").Bool() {
					resolve.Invoke()
					return nil
				}

				packet := result.Get("value")

				// Process the packet (Uint8Array)
				dlen := packet.Length()
				bin := make([]byte, dlen)
				js.CopyBytesToGo(bin, packet)
				fmt.Printf("Received packet: %v\n", bin)

				js.Global().Call("setTimeout", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
					iteratePackets()
					return nil
				}), 0)
				return nil
			})).Call("catch", reject)
		}

		js.Global().Call("setTimeout", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			iteratePackets()
			return nil
		}), 0)

		return nil
	}))

	promise.Call("then", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		fmt.Println("Finished iterating over packet stream")
		return nil
	})).Call("catch", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		fmt.Printf("Error iterating over packet stream: %v\n", args[0])
		return nil
	}))

	return nil
}
