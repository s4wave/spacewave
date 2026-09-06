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
	// globalOpenStreamToWebRuntime is the name of the global function which
	// opens a stream to the web runtime:
	//
	// BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME?: (
	//   onMessage: (message: Uint8Array) => void,
	//   onClose: (errMsg?: string) => void,
	//   onResolve: (sink: { push: (message: Uint8Array) => void, end: () => void }) => void,
	//   onReject: (errMsg: string) => void,
	// ) => void
	globalOpenStreamToWebRuntime = "BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME"
	// globalSetAcceptStream is the name of the global function which sets the
	// incoming stream accept callback:
	//
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

		// Construct the deferred writer around the packet callback release.
		packetCallbacks := &jsStreamCallbacks{}
		packetWriter := newDeferredPushablePacketWriter(packetCallbacks.Release)

		// Open the stream in the background.
		go openPushableStream(ctx, openStreamFunc, msgHandler, closeHandler, packetWriter, packetCallbacks)

		// Close the writer when the context ends.
		go func() {
			<-ctx.Done()
			_ = packetWriter.Close()
		}()
		return packetWriter, nil
	}
}

// openPushableStream invokes the open stream bridge and wires the packet and
// promise callbacks to the pending writer.
func openPushableStream(
	ctx context.Context,
	openStreamFunc js.Value,
	msgHandler srpc.PacketDataHandler,
	closeHandler srpc.CloseHandler,
	packetWriter *deferredPushablePacketWriter,
	packetCallbacks *jsStreamCallbacks,
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

	// Refuse to open a stream after cancellation.
	if err := ctx.Err(); err != nil {
		packetWriter.fail(err)
		closeHandler(err)
		return
	}

	// Serialize packets from the JS message callback through one handler.
	packetHandler := newSerialPacketDataHandler(msgHandler, closeHandler, packetCallbacks.Release)

	// jsOnMessage copies each packet message into Go-owned storage and
	// forwards it to the serialized handler.
	// (message: Uint8Array) => void
	jsOnMessage := js.FuncOf(func(this js.Value, args []js.Value) any {
		defer recoverJSCallback("handle stream packet", func(err error) {
			packetHandler.Fail(err)
		})
		// Copy the packet into Go-owned storage: the handler may retain the
		// slice, so it must never alias the JS Uint8Array's backing storage.
		packet := args[0]
		bin := make([]byte, packet.Length())
		js.CopyBytesToGo(bin, packet)

		packetHandler.Handle(bin)
		return nil
	})
	// jsOnClose closes the handler, propagating the remote error message.
	// (errMsg?: string) => void
	jsOnClose := js.FuncOf(func(this js.Value, args []js.Value) any {
		defer recoverJSCallback("handle stream close", packetHandler.Fail)
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

		packetHandler.Close(err)
		return nil
	})
	// Register the packet callbacks; close the handler when already released.
	if !packetCallbacks.Set(jsOnMessage, jsOnClose) {
		packetHandler.Close(io.ErrClosedPipe)
		return
	}

	// promiseCallbacks releases the promise pair exactly once, from whichever
	// of jsThen or jsCatch runs first.
	promiseCallbacks := &jsStreamCallbacks{}

	// jsThen resolves the pending writer with the stream sink.
	jsThen := js.FuncOf(func(this js.Value, args []js.Value) any {
		defer recoverJSCallback("resolve stream sink", func(err error) {
			promiseCallbacks.Release()
			packetWriter.fail(err)
			closeHandler(err)
		})

		// Release the promise pair before delivering the sink.
		sink := args[0]
		promiseCallbacks.Release()

		deferTinyGoCallbackTask(func() {
			packetWriter.resolve(sink)
		})
		return nil
	})

	// jsCatch fails the pending writer with the rejection error.
	jsCatch := js.FuncOf(func(this js.Value, args []js.Value) any {
		defer recoverJSCallback("reject stream sink", func(err error) {
			promiseCallbacks.Release()
			packetWriter.fail(err)
			closeHandler(err)
		})

		// Release the promise pair and classify the rejection reason.
		promiseCallbacks.Release()
		var err error
		if len(args) == 0 || args[0].IsUndefined() || args[0].IsNull() {
			err = errors.New("open stream rejected")
		} else if args[0].Type() == js.TypeString {
			err = errors.New(strings.TrimPrefix(args[0].String(), "Error: "))
		} else {
			err = errors.Errorf("open stream rejected: %v", args[0])
		}

		deferTinyGoCallbackTask(func() {
			packetWriter.fail(err)
			closeHandler(err)
		})
		return nil
	})

	// Register the promise pair before invoking the bridge.
	promiseCallbacks.Set(jsThen, jsCatch)
	if packetWriter.isClosed() {
		promiseCallbacks.Release()
		packetCallbacks.Release()
		return
	}

	// Invoke the bridge to open the stream.
	openStreamFunc.Invoke(jsOnMessage, jsOnClose, jsThen, jsCatch)
}

// releaseJSFunc releases a js.Func, deferring the release on TinyGo so the
// callback frame has returned to the JS runtime first.
func releaseJSFunc(fn js.Func) {
	if runtime.Compiler != "tinygo" {
		fn.Release()
		return
	}
	// TinyGo's syscall/js callback frame is still live while the callback is
	// running. Release from a later scheduler task so the callback has returned
	// to the JS runtime before TinyGo mutates its function table.
	deferTinyGoCallbackTask(func() {
		fn.Release()
	})
}

// deferTinyGoCallbackTask runs fn on a later scheduler task on TinyGo and
// immediately otherwise.
func deferTinyGoCallbackTask(fn func()) {
	if runtime.Compiler != "tinygo" {
		fn()
		return
	}
	time.AfterFunc(0, fn)
}

// jsStreamCallbacks releases a callback pair once when its caller requests
// cleanup, including when Release precedes Set.
type jsStreamCallbacks struct {
	// mtx guards the fields below.
	mtx sync.Mutex

	// first is the first callback retained until Release.
	first js.Func
	// second is the second callback retained until Release.
	second js.Func

	// ready indicates whether the callbacks were set.
	ready bool
	// released prevents registration and selects one cleanup caller.
	released bool
}

// Set registers the callback pair and reports whether they were stored.
func (c *jsStreamCallbacks) Set(first, second js.Func) bool {
	c.mtx.Lock()
	if c.released {
		c.mtx.Unlock()
		first.Release()
		second.Release()
		return false
	}
	c.first = first
	c.second = second
	c.ready = true
	c.mtx.Unlock()
	return true
}

// Release releases the registered callback pair exactly once.
func (c *jsStreamCallbacks) Release() {
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
	first := c.first
	second := c.second
	c.mtx.Unlock()

	releaseJSFunc(first)
	releaseJSFunc(second)
}

// recoverJSCallback converts a recovered panic into an error passed to onErr.
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

// serialPacketDataHandler owns srpc.OpenStreamFunc's non-concurrent msgHandler
// contract. TinyGo can re-enter js.FuncOf from later JS tasks while an earlier
// callback goroutine is blocked, so callbacks enqueue packets and one Go
// goroutine drains them.
type serialPacketDataHandler struct {
	// msgHandler handles each packet in delivery order.
	msgHandler srpc.PacketDataHandler
	// closeHandler is invoked exactly once when the stream finishes.
	closeHandler srpc.CloseHandler
	// releaseFn releases the stream callbacks.
	releaseFn func()

	// mtx guards the fields below.
	mtx sync.Mutex
	// notify wakes the drain goroutine; buffered capacity one.
	notify chan struct{}
	// queue holds packets awaiting the serialized handler.
	queue []serialPacketData
	// closed rejects new packets while the remaining queue drains.
	closed bool
	// closeErr is the error passed to closeHandler.
	closeErr error
	// finished selects the caller responsible for closeHandler and releaseFn.
	finished bool
	// scheduled indicates whether a TinyGo drain task is pending.
	scheduled bool
}

// serialPacketData carries one queued packet and its delivery acknowledgement.
type serialPacketData struct {
	// data is the packet bytes delivered to the message handler.
	data []byte
	// handled acknowledges delivery to the JS callback, if any.
	handled func(error)
}

// newSerialPacketDataHandler constructs the handler and starts its drain
// goroutine. releaseFn is invoked exactly once when the stream finishes.
func newSerialPacketDataHandler(
	msgHandler srpc.PacketDataHandler,
	closeHandler srpc.CloseHandler,
	releaseFn func(),
) *serialPacketDataHandler {
	h := &serialPacketDataHandler{
		msgHandler:   msgHandler,
		closeHandler: closeHandler,
		releaseFn:    releaseFn,
		notify:       make(chan struct{}, 1),
	}
	go h.run()
	return h
}

// Handle queues a packet without an acknowledgement callback.
func (h *serialPacketDataHandler) Handle(data []byte) {
	h.HandleWithResult(data, nil)
}

// HandleWithResult queues a packet and invokes handled with the delivery
// result after the message handler returns.
func (h *serialPacketDataHandler) HandleWithResult(data []byte, handled func(error)) {
	h.mtx.Lock()
	if h.closed {
		h.mtx.Unlock()
		if handled != nil {
			handled(io.ErrClosedPipe)
		}
		return
	}
	h.queue = append(h.queue, serialPacketData{data: data, handled: handled})
	h.scheduleLocked()
	h.mtx.Unlock()
}

// Close finishes the stream with err after the queued packets drain.
func (h *serialPacketDataHandler) Close(err error) {
	h.mtx.Lock()
	if h.closed {
		h.mtx.Unlock()
		return
	}
	h.closed = true
	h.closeErr = err
	h.scheduleLocked()
	h.mtx.Unlock()
}

// Fail finishes the stream immediately, acknowledging queued packets with err.
func (h *serialPacketDataHandler) Fail(err error) {
	h.mtx.Lock()
	if h.closed && len(h.queue) == 0 {
		h.mtx.Unlock()
		return
	}
	h.closed = true
	h.closeErr = err
	pending := h.queue
	h.queue = nil
	h.scheduleLocked()
	h.mtx.Unlock()

	for _, item := range pending {
		if item.handled != nil {
			item.handled(err)
		}
	}
	h.finish(err)
}

// finish invokes closeHandler and releaseFn exactly once.
func (h *serialPacketDataHandler) finish(err error) {
	h.mtx.Lock()
	if h.finished {
		h.mtx.Unlock()
		return
	}
	h.finished = true
	h.mtx.Unlock()

	h.closeHandler(err)
	h.releaseFn()
}

// run drains queued packets in delivery order until the stream closes.
func (h *serialPacketDataHandler) run() {
	for {
		h.mtx.Lock()
		for len(h.queue) == 0 && !h.closed {
			h.mtx.Unlock()
			<-h.notify
			h.mtx.Lock()
		}
		if len(h.queue) != 0 {
			item := h.queue[0]
			copy(h.queue, h.queue[1:])
			h.queue[len(h.queue)-1] = serialPacketData{}
			h.queue = h.queue[:len(h.queue)-1]
			h.mtx.Unlock()
			if err := h.msgHandler(item.data); err != nil {
				if item.handled != nil {
					item.handled(err)
				}
				h.Fail(err)
				return
			}
			if item.handled != nil {
				item.handled(nil)
			}
			continue
		}
		err := h.closeErr
		h.mtx.Unlock()

		h.finish(err)
		return
	}
}

// signalLocked wakes the drain goroutine without blocking the caller.
func (h *serialPacketDataHandler) signalLocked() {
	select {
	case h.notify <- struct{}{}:
	default:
	}
}

// scheduleLocked wakes the drain goroutine, deferring the wake through a
// scheduler task on TinyGo so callbacks never block.
func (h *serialPacketDataHandler) scheduleLocked() {
	if runtime.Compiler != "tinygo" {
		h.signalLocked()
		return
	}
	if h.scheduled {
		return
	}
	h.scheduled = true
	time.AfterFunc(0, func() {
		h.mtx.Lock()
		h.scheduled = false
		h.signalLocked()
		h.mtx.Unlock()
	})
}

// deferredPushablePacketWriter queues packets until the JS stream resolves
// and hands writes to the resolved PushablePacketWriter.
type deferredPushablePacketWriter struct {
	// mtx guards the fields below.
	mtx sync.Mutex

	// writer is the resolved writer, nil until the stream resolves.
	writer *PushablePacketWriter
	// queued holds packets written before the stream resolved.
	queued [][]byte
	// closed indicates whether the writer finished.
	closed bool
	// err is the error returned by writes after close.
	err error
	// releaseFn releases the stream callbacks.
	releaseFn func()
}

// newDeferredPushablePacketWriter constructs the writer.
// releaseFn is invoked exactly once when the writer finishes.
func newDeferredPushablePacketWriter(releaseFn func()) *deferredPushablePacketWriter {
	return &deferredPushablePacketWriter{
		releaseFn: releaseFn,
	}
}

// WritePacket marshals and queues or writes a packet.
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

// isClosed reports whether the writer already finished.
func (w *deferredPushablePacketWriter) isClosed() bool {
	w.mtx.Lock()
	closed := w.closed
	w.mtx.Unlock()
	return closed
}

// Close finishes the writer, closing the resolved writer if any.
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
	w.releaseFn()
	return err
}

// resolve hands the queued writes to a writer for the resolved pushable.
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
			w.releaseFn()
			return
		}
	}
	w.queued = nil
	w.writer = writer
	w.mtx.Unlock()
}

// fail finishes the writer with err.
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
	w.releaseFn()
}

// GlobalWasmPluginIo gets the message port defined by plugin-wasm.ts.
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
	if runtime.Compiler == "tinygo" {
		return newTinyGoPluginOpenStream(ctx, msgHandler, closeHandler)
	}
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
	if runtime.Compiler == "tinygo" {
		setTinyGoPluginAcceptStreams(ctx, invoker)
		return
	}

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
