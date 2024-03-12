//go:build js
// +build js

package web_entrypoint_browser

import (
	"context"
	"syscall/js"

	message_port "github.com/aperturerobotics/bldr/web/entrypoint/browser/message-port"
	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/sirupsen/logrus"
)

// Runtime is the alias to the remote runtime type.
type Runtime = web_runtime.Remote

// NewRuntime constructs the remote web runtime.
func NewRuntime(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	handler web_runtime.WebRuntimeHandler,
	runtimeID string,
	messagePort js.Value,
) (r *Runtime, rerr error) {
	defer func() {
		if err := recover(); err != nil {
			rerr = err.(error)
		}
	}()

	// handle incoming streams from the message port
	messagePort.Set("onmessage", js.FuncOf(
		func(t js.Value, args []js.Value) interface{} {
			if len(args) < 1 || r == nil || ctx.Err() != nil {
				return nil
			}

			msgEvent := args[0]
			dat := msgEvent.Get("data")
			if dat.String() != "open-stream" {
				// ignore
				return nil
			}

			// we expect a message port passed in the event
			ports := msgEvent.Get("ports")
			port := ports.Index(0)

			// construct the port & start receiving messages
			portDuplex := message_port.NewMessagePort(port)
			portStream := message_port.NewMessagePortPacketStream(portDuplex)

			// accept the port -> the rpc server
			serverRPC := srpc.NewServerRPC(ctx, r.GetRpcServer().GetInvoker(), portStream)
			go portStream.ReadPump(ctx, serverRPC.HandlePacketData, serverRPC.HandleStreamClose)

			return nil
		},
	))

	rpcClient := srpc.NewClient(func(
		ctx context.Context,
		msgHandler srpc.PacketDataHandler,
		closeHandler srpc.CloseHandler,
	) (srpc.PacketWriter, error) {
		localPort, remotePort := message_port.NewMessageChannel()
		_ = messagePort.Call("postMessage", "open-stream", []interface{}{remotePort})
		duplex := message_port.NewMessagePort(localPort)
		stream := message_port.NewMessagePortPacketStream(duplex)
		go stream.ReadPump(ctx, msgHandler, closeHandler)
		return stream, nil
	})

	r, rerr = web_runtime.NewRemote(le, b, handler, runtimeID, rpcClient, nil)
	if rerr != nil {
		return nil, rerr
	}

	messagePort.Call("start")
	return r, nil
}
