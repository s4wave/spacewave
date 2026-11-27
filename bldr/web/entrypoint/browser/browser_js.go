//go:build js

package web_entrypoint_browser

import (
	"context"
	"errors"
	"syscall/js"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	message_port "github.com/s4wave/spacewave/bldr/web/entrypoint/browser/message-port"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	"github.com/sirupsen/logrus"
)

// ControllerID is the browser runtime controller ID.
const ControllerID = "bldr/web/entrypoint/browser"

// Version is the version of the runtime implementation.
var Version = controller.MustParseVersion("0.0.1")

// ReadInitMessage reads the bldr init message from the global.
//
// configured by runtime-wasm.ts
func ReadInitMessage() (*web_runtime.WebRuntimeHostInit, error) {
	// Require the initialization message installed by the loader.
	wasmInit := js.Global().Get("BLDR_INIT")
	if wasmInit.IsUndefined() {
		return nil, errors.New("init information was not defined")
	}

	// Copy and decode the initialization message.
	bin := make([]byte, wasmInit.Length())
	js.CopyBytesToGo(bin, wasmInit)
	v := &web_runtime.WebRuntimeHostInit{}
	if err := v.UnmarshalVT(bin); err != nil {
		return nil, err
	}
	return v, nil
}

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
	// Return JavaScript exceptions as runtime construction errors.
	defer func() {
		if err := recover(); err != nil {
			rerr = err.(error)
		}
	}()

	// handle incoming streams from the message port
	messagePort.Set("onmessage", js.FuncOf(
		func(t js.Value, args []js.Value) interface{} {
			// Require an active runtime and a message event.
			if len(args) < 1 || r == nil || ctx.Err() != nil {
				return nil
			}

			// Ignore messages that do not request an RPC stream.
			msgEvent := args[0]
			dat := msgEvent.Get("data")
			if dat.String() != "open-stream" {
				// ignore
				return nil
			}

			// Receive the message port supplied for this RPC stream.
			ports := msgEvent.Get("ports")
			port := ports.Index(0)

			// construct the port & start receiving messages
			portDuplex := message_port.NewMessagePort(port)
			portStream := message_port.NewMessagePortPacketStream(portDuplex)

			// accept the port -> the rpc server
			serverRPC := r.NewServerRPC(ctx, portStream)
			go portStream.ReadPump(ctx, serverRPC.HandlePacketData, serverRPC.HandleStreamClose)

			return nil
		},
	))

	// Open each outbound RPC over a new message channel.
	rpcClient := srpc.NewClient(func(
		ctx context.Context,
		msgHandler srpc.PacketDataHandler,
		closeHandler srpc.CloseHandler,
	) (srpc.PacketWriter, error) {
		// Transfer the remote port and pump packets on the local port.
		localPort, remotePort := message_port.NewMessageChannel()
		_ = messagePort.Call("postMessage", "open-stream", []interface{}{remotePort})
		duplex := message_port.NewMessagePort(localPort)
		stream := message_port.NewMessagePortPacketStream(duplex)
		go stream.ReadPump(ctx, msgHandler, closeHandler)
		return stream, nil
	})

	// Bind runtime services to the message-port client.
	r, rerr = web_runtime.NewRemote(le, b, handler, runtimeID, rpcClient, nil)
	if rerr != nil {
		return nil, rerr
	}

	// Start delivering messages after the runtime is ready.
	messagePort.Call("start")
	return r, nil
}
