//go:build js

package web_runtime_wasm

import (
	"context"
	"strings"
	"syscall/js"

	message_port "github.com/aperturerobotics/bldr/web/entrypoint/browser/message-port"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
)

const (
	// BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME?: (
	//   onMessage: (message: Uint8Array) => void,
	//   onClose: (errMsg?: string) => void,
	// ) => Promise<Pushable<Uint8Array>>
	globalOpenStreamToWebRuntime = "BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME"
	// BLDR_PLUGIN_SET_ACCEPT_STREAM?: (acceptStream: () => MessagePort) => void
	globalSetAcceptStream = "BLDR_PLUGIN_SET_ACCEPT_STREAM"
)

// NewPushableOpenStream builds an srpc open stream function with a pushable func.
//
// See BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME
func NewPushableOpenStream(openStreamFunc js.Value) srpc.OpenStreamFunc {
	return func(
		ctx context.Context,
		msgHandler srpc.PacketDataHandler,
		closeHandler srpc.CloseHandler,
	) (_ srpc.PacketWriter, err error) {
		defer func() {
			if e := recover(); e != nil {
				switch recovered := e.(type) {
				case error:
					err = errors.Wrap(recovered, "invoke open stream to web runtime")
				default:
					err = errors.Errorf("invoke open stream to web runtime: %v", recovered)
				}
			}
		}()

		// (message: Uint8Array) => void
		jsOnMessage := js.FuncOf(func(this js.Value, args []js.Value) any {
			// copy packet from Uint8Array to []byte
			packet := args[0]
			dlen := packet.Length()
			bin := make([]byte, dlen)
			js.CopyBytesToGo(bin, packet)

			// call handler and handle error
			if err := msgHandler(bin); err != nil {
				closeHandler(err)
			}

			return nil
		})
		// (errMsg?: string) => void,
		jsOnClose := js.FuncOf(func(this js.Value, args []js.Value) any {
			var errMsg string
			if len(args) > 0 {
				errMsgVal := args[0]
				if !errMsgVal.IsUndefined() && errMsgVal.Type() == js.TypeString {
					errMsg = errMsgVal.String()
				}
			}

			var err error
			if len(errMsg) != 0 {
				err = errors.New(errMsg)
			}

			closeHandler(err)
			return nil
		})

		sinkPromise := openStreamFunc.Invoke(jsOnMessage, jsOnClose)
		errCh := make(chan error, 1)
		doneCh := make(chan srpc.PacketWriter, 1)
		sinkPromise.Call("then", js.FuncOf(func(this js.Value, args []js.Value) any {
			select {
			case <-ctx.Done():
				args[0].Call("end")
				return nil
			case doneCh <- NewPushablePacketWriter(args[0]):
				return nil
			}
		})).Call("catch", js.FuncOf(func(this js.Value, args []js.Value) any {
			if args[0].Type() == js.TypeObject {
				// Error
				errCh <- errors.New(strings.TrimPrefix(args[0].Call("toString").String(), "Error: "))
			} else {
				// String
				errCh <- errors.New(args[0].String())
			}
			return nil
		}))

		select {
		case prw := <-doneCh:
			return prw, nil
		case err := <-errCh:
			return nil, err
		}
	}
}

// GlobalWasmPluginIo gets the message port defined by plugin-wasm.ts
func GlobalWasmPluginIo() (*WasmPluginIo, error) {
	global := js.Global()
	if global.IsUndefined() {
		return nil, errors.New("js: global is undefined")
	}

	return NewWasmPluginIo(
		global.Get(globalOpenStreamToWebRuntime),
		global.Get(globalSetAcceptStream),
	)
}

// WasmPluginIo manages opening outgoing rpc streams and accepting incoming streams.
// Communicates with plugin-wasm.ts.
type WasmPluginIo struct {
	// openStreamName is the global name for the outgoing stream bridge.
	openStreamName string
	// setAcceptStreamName is the global name for the incoming stream bridge.
	setAcceptStreamName string
}

// NewWasmPluginIo constructs the wasm plugin i/o.
//
// openStreamToWebRuntime: see BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME
// setAcceptStream: see BLDR_PLUGIN_SET_ACCEPT_STREAM
func NewWasmPluginIo(openStreamToWebRuntime, setAcceptStream js.Value) (*WasmPluginIo, error) {
	if setAcceptStream.IsUndefined() || setAcceptStream.Type() != js.TypeFunction {
		return nil, errors.Errorf("js: %v is not a function", globalSetAcceptStream)
	}
	if openStreamToWebRuntime.IsUndefined() || openStreamToWebRuntime.Type() != js.TypeFunction {
		return nil, errors.Errorf("js: %v is not a function", globalOpenStreamToWebRuntime)
	}
	return &WasmPluginIo{
		openStreamName:      globalOpenStreamToWebRuntime,
		setAcceptStreamName: globalSetAcceptStream,
	}, nil
}

// getGlobalFunc resolves a global function by name.
func getGlobalFunc(name string) (js.Value, error) {
	fn := js.Global().Get(name)
	if fn.IsUndefined() || fn.IsNull() || fn.Type() != js.TypeFunction {
		return js.Undefined(), errors.Errorf("js: %v is not a function", name)
	}
	return fn, nil
}

// OpenStream opens an RPC stream via openStreamToWebRuntime.
func (p *WasmPluginIo) OpenStream(
	ctx context.Context,
	msgHandler srpc.PacketDataHandler,
	closeHandler srpc.CloseHandler,
) (srpc.PacketWriter, error) {
	openStreamFunc, err := getGlobalFunc(p.openStreamName)
	if err != nil {
		return nil, err
	}
	return NewPushableOpenStream(openStreamFunc)(ctx, msgHandler, closeHandler)
}

// BuildClient builds a new srpc.Client with the open stream func.
func (p *WasmPluginIo) BuildClient() srpc.Client {
	return srpc.NewClient(p.OpenStream)
}

// SetAcceptStreams sets the function to call to accept incoming streams.
func (p *WasmPluginIo) SetAcceptStreams(ctx context.Context, invoker srpc.Invoker) {
	// acceptStreamFn is () => MessagePort
	acceptStreamFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		localPort, remotePort := message_port.NewMessageChannel()
		duplex := message_port.NewMessagePort(localPort)
		stream := message_port.NewMessagePortPacketStream(duplex)

		serverRPC := srpc.NewServerRPC(ctx, invoker, stream)
		go stream.ReadPump(ctx, serverRPC.HandlePacketData, serverRPC.HandleStreamClose)
		return remotePort
	})
	setAcceptStream, err := getGlobalFunc(p.setAcceptStreamName)
	if err != nil {
		panic(err)
	}
	setAcceptStream.Invoke(acceptStreamFn)
}
