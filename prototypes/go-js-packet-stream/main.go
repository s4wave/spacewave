package main

import (
	"context"
	"fmt"
	"syscall/js"

	web_runtime_wasm "github.com/aperturerobotics/bldr/web/runtime/wasm"
	"github.com/aperturerobotics/starpc/srpc"
)

func main() {
	globalOpenStream := js.Global().Get("openStream")
	var openStreamFunc srpc.OpenStreamFunc = web_runtime_wasm.NewPushableOpenStream(globalOpenStream)

	ctx := context.Background()
	prw, err := openStreamFunc(ctx, func(data []byte) error {
		fmt.Printf("stream message handler: %v\n", data)
		return nil
	}, func(closeErr error) {
		fmt.Printf("stream close handler: %v\n", closeErr)
	})
	if err != nil {
		fmt.Printf("stream open error: %v\n", err)
	}

	for i := byte(0); i < 10; i++ {
		err := prw.WritePacket(&srpc.Packet{
			Body: &srpc.Packet_CallData{
				CallData: &srpc.CallData{
					Data: []byte{i, i + 1, i + 2},
				},
			},
		})
		if err != nil {
			fmt.Printf("stream write packet error: %v\n", err)
		}
	}
	if err := prw.Close(); err != nil {
		fmt.Printf("stream close error: %v\n", err)
	}

	<-ctx.Done()
}
