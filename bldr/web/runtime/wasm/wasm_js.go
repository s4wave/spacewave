//go:build js

package web_runtime_wasm

import (
	"context"
	"io"
	"runtime"
	"strings"
	"sync"
	"syscall/js"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	message_port "github.com/s4wave/spacewave/bldr/web/entrypoint/browser/message-port"
)

const (
	// BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME?: (
	//   onMessage: (message: Uint8Array) => void,
	//   onClose: (errMsg?: string) => void,
	//   onResolve: (sink: { push: (message: Uint8Array) => void, end: () => void }) => void,
	//   onReject: (errMsg: string) => void,
	// ) => void
	globalOpenStreamToWebRuntime = "BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME"
	// BLDR_PLUGIN_SET_ACCEPT_STREAM?: (acceptStream: (localPort: MessagePort) => void) => void
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
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		packetCallbacks := &jsStreamPacketCallbacks{}
		packetWriter := newDeferredPushablePacketWriter(packetCallbacks.Release)
		go openPushableStream(ctx, openStreamFunc, msgHandler, closeHandler, packetWriter, packetCallbacks)
		go func() {
			<-ctx.Done()
			_ = packetWriter.Close()
		}()
		return packetWriter, nil
	}
}

func openPushableStream(
	ctx context.Context,
	openStreamFunc js.Value,
	msgHandler srpc.PacketDataHandler,
	closeHandler srpc.CloseHandler,
	packetWriter *deferredPushablePacketWriter,
	packetCallbacks *jsStreamPacketCallbacks,
) {
	defer func() {
		if e := recover(); e != nil {
			var err error
			switch recovered := e.(type) {
			case error:
				err = errors.Wrap(recovered, "invoke open stream to web runtime")
			default:
				err = errors.Errorf("invoke open stream to web runtime: %v", recovered)
			}
			packetWriter.fail(err)
			closeHandler(err)
		}
	}()

	if err := ctx.Err(); err != nil {
		packetWriter.fail(err)
		closeHandler(err)
		return
	}

	// (message: Uint8Array) => void
	jsOnMessage := js.FuncOf(func(this js.Value, args []js.Value) any {
		defer recoverJSCallback("handle stream packet", func(err error) {
			closeHandler(err)
			packetCallbacks.Release()
		})
		// copy packet from Uint8Array to []byte
		packet := args[0]
		dlen := packet.Length()
		bin := make([]byte, dlen)
		for i := 0; i < dlen; i++ {
			bin[i] = byte(packet.Index(i).Int())
		}

		// call handler and handle error
		if err := msgHandler(bin); err != nil {
			closeHandler(err)
			packetCallbacks.Release()
		}

		return nil
	})
	// (errMsg?: string) => void,
	jsOnClose := js.FuncOf(func(this js.Value, args []js.Value) any {
		defer recoverJSCallback("handle stream close", closeHandler)
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
		packetCallbacks.Release()
		return nil
	})
	if !packetCallbacks.Set(jsOnMessage, jsOnClose) {
		return
	}

	var releasePromiseCallbacks sync.Once
	var jsThen js.Func
	var jsCatch js.Func
	jsThen = js.FuncOf(func(this js.Value, args []js.Value) any {
		defer recoverJSCallback("resolve stream sink", func(err error) {
			releasePromiseCallbacks.Do(func() {
				releaseJSFunc(jsThen)
				releaseJSFunc(jsCatch)
			})
			packetWriter.fail(err)
			closeHandler(err)
		})
		releasePromiseCallbacks.Do(func() {
			releaseJSFunc(jsThen)
			releaseJSFunc(jsCatch)
		})
		packetWriter.resolve(args[0])
		return nil
	})
	jsCatch = js.FuncOf(func(this js.Value, args []js.Value) any {
		defer recoverJSCallback("reject stream sink", func(err error) {
			releasePromiseCallbacks.Do(func() {
				releaseJSFunc(jsThen)
				releaseJSFunc(jsCatch)
			})
			packetWriter.fail(err)
			closeHandler(err)
		})
		releasePromiseCallbacks.Do(func() {
			releaseJSFunc(jsThen)
			releaseJSFunc(jsCatch)
		})
		var err error
		if len(args) == 0 || args[0].IsUndefined() || args[0].IsNull() {
			err = errors.New("open stream rejected")
		} else if args[0].Type() == js.TypeString {
			err = errors.New(strings.TrimPrefix(args[0].String(), "Error: "))
		} else {
			err = errors.Errorf("open stream rejected: %v", args[0])
		}
		packetWriter.fail(err)
		closeHandler(err)
		return nil
	})
	if packetWriter.isClosed() {
		releasePromiseCallbacks.Do(func() {
			releaseJSFunc(jsThen)
			releaseJSFunc(jsCatch)
		})
		packetCallbacks.Release()
		return
	}
	openStreamFunc.Invoke(jsOnMessage, jsOnClose, jsThen, jsCatch)
}

func releaseJSFunc(fn js.Func) {
	if runtime.Compiler != "tinygo" {
		fn.Release()
		return
	}
	go func() {
		// TinyGo's syscall/js callback frame is still live while the callback
		// is running. Releasing from that same stack can corrupt the JS value
		// table and make later unrelated js.Value calls trap.
		time.Sleep(time.Millisecond)
		fn.Release()
	}()
}

type jsStreamPacketCallbacks struct {
	mtx       sync.Mutex
	onMessage js.Func
	onClose   js.Func
	ready     bool
	released  bool
}

func (c *jsStreamPacketCallbacks) Set(onMessage, onClose js.Func) bool {
	c.mtx.Lock()
	if c.released {
		c.mtx.Unlock()
		onMessage.Release()
		onClose.Release()
		return false
	}
	c.onMessage = onMessage
	c.onClose = onClose
	c.ready = true
	c.mtx.Unlock()
	return true
}

func (c *jsStreamPacketCallbacks) Release() {
	c.mtx.Lock()
	if c.released {
		c.mtx.Unlock()
		return
	}
	c.released = true
	if !c.ready {
		c.mtx.Unlock()
		return
	}
	onMessage := c.onMessage
	onClose := c.onClose
	c.mtx.Unlock()

	releaseJSFunc(onMessage)
	releaseJSFunc(onClose)
}

func recoverJSCallback(label string, onErr func(error)) {
	if e := recover(); e != nil {
		var err error
		switch recovered := e.(type) {
		case error:
			err = errors.Wrap(recovered, label)
		default:
			err = errors.Errorf("%s: %v", label, recovered)
		}
		onErr(err)
	}
}

type deferredPushablePacketWriter struct {
	mtx       sync.Mutex
	writer    *PushablePacketWriter
	queued    [][]byte
	closed    bool
	err       error
	releaseFn func()
	release   sync.Once
}

func newDeferredPushablePacketWriter(releaseFn func()) *deferredPushablePacketWriter {
	return &deferredPushablePacketWriter{
		releaseFn: releaseFn,
	}
}

func (w *deferredPushablePacketWriter) WritePacket(pkt *srpc.Packet) error {
	data, err := pkt.MarshalVT()
	if err != nil {
		return err
	}

	w.mtx.Lock()
	defer w.mtx.Unlock()
	if w.closed {
		if w.err != nil {
			return w.err
		}
		return io.ErrClosedPipe
	}
	if w.writer == nil {
		w.queued = append(w.queued, data)
		return nil
	}
	return w.writer.WritePacketData(data)
}

func (w *deferredPushablePacketWriter) isClosed() bool {
	w.mtx.Lock()
	closed := w.closed
	w.mtx.Unlock()
	return closed
}

func (w *deferredPushablePacketWriter) Close() error {
	w.mtx.Lock()
	if w.closed {
		w.mtx.Unlock()
		return nil
	}
	w.closed = true
	w.err = io.ErrClosedPipe
	w.queued = nil
	writer := w.writer
	w.mtx.Unlock()

	var err error
	if writer != nil {
		err = writer.Close()
	}
	w.release.Do(w.releaseFn)
	return err
}

func (w *deferredPushablePacketWriter) resolve(pushable js.Value) {
	writer := NewPushablePacketWriter(pushable)

	w.mtx.Lock()
	if w.closed {
		w.mtx.Unlock()
		_ = writer.Close()
		return
	}

	for _, data := range w.queued {
		if err := writer.WritePacketData(data); err != nil {
			w.closed = true
			w.err = err
			w.queued = nil
			w.mtx.Unlock()
			_ = writer.Close()
			w.release.Do(w.releaseFn)
			return
		}
	}
	w.queued = nil
	w.writer = writer
	w.mtx.Unlock()
}

func (w *deferredPushablePacketWriter) fail(err error) {
	if err == nil {
		err = io.ErrClosedPipe
	}

	w.mtx.Lock()
	if w.closed {
		w.mtx.Unlock()
		return
	}
	w.closed = true
	w.err = err
	w.queued = nil
	writer := w.writer
	w.mtx.Unlock()

	if writer != nil {
		_ = writer.Close()
	}
	w.release.Do(w.releaseFn)
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
	var activeMtx sync.Mutex
	activeStreams := map[*message_port.MessagePort]struct{}{}
	addActiveStream := func(stream *message_port.MessagePort) {
		activeMtx.Lock()
		activeStreams[stream] = struct{}{}
		activeMtx.Unlock()
	}
	removeActiveStream := func(stream *message_port.MessagePort) {
		activeMtx.Lock()
		delete(activeStreams, stream)
		activeMtx.Unlock()
	}
	closeActiveStreams := func() {
		activeMtx.Lock()
		streams := activeStreams
		activeStreams = map[*message_port.MessagePort]struct{}{}
		activeMtx.Unlock()
		for stream := range streams {
			_ = stream.Close()
		}
	}

	// acceptStreamFn is (localPort: MessagePort) => void
	acceptStreamFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		if ctx.Err() != nil {
			return nil
		}
		if len(args) == 0 || args[0].IsUndefined() || args[0].IsNull() {
			panic("accept stream missing MessagePort")
		}

		duplex := message_port.NewMessagePort(args[0])
		addActiveStream(duplex)
		stream := message_port.NewMessagePortPacketStream(duplex)

		serverRPC := srpc.NewServerRPC(ctx, invoker, stream)
		go func() {
			defer removeActiveStream(duplex)
			defer duplex.Close()
			stream.ReadPump(ctx, serverRPC.HandlePacketData, serverRPC.HandleStreamClose)
		}()
		return nil
	})
	setAcceptStream, err := getGlobalFunc(p.setAcceptStreamName)
	if err != nil {
		panic(err)
	}
	setAcceptStream.Invoke(acceptStreamFn)
	go func() {
		<-ctx.Done()
		setAcceptStream.Invoke(js.Undefined())
		releaseJSFunc(acceptStreamFn)
		closeActiveStreams()
	}()
}
