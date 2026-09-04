//go:build js && bldr_cloudflare

package plugin_entrypoint

import (
	"context"
	"io"
	"runtime"
	"sync"
	"syscall/js"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	web_runtime_wasm "github.com/s4wave/spacewave/bldr/web/runtime/wasm"
)

const (
	// BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME: (
	//   onMessage: (message: Uint8Array) => void,
	//   onClose: (errMsg?: string) => void,
	//   onResolve: (sink: { push: (message: Uint8Array) => void, end: () => void }) => void,
	//   onReject: (errMsg: string) => void,
	// ) => void
	globalOpenStreamToWebRuntime = "BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME"
	// BLDR_PLUGIN_SET_ACCEPT_STREAM_WORKERS: (acceptStream: (openStreamFunc: (onMessage, onClose, onResolve, onReject) => void) => void) => void
	globalSetAcceptStream = "BLDR_PLUGIN_SET_ACCEPT_STREAM_WORKERS"
)

// newPluginTransport resolves the plugin transport for the current runtime.
// For Cloudflare Workers js/wasm builds this obtains the IO from the Worker
// host runtime globals (see plugin-goscript-cloudflare.ts).
func newPluginTransport() (pluginTransport, error) {
	global := js.Global()
	if global.IsUndefined() {
		return nil, errors.New("js: global is undefined")
	}

	return NewCloudflarePluginIo(
		global.Get(globalOpenStreamToWebRuntime),
		global.Get(globalSetAcceptStream),
	)
}

// CloudflarePluginIo manages opening outgoing rpc streams and accepting
// incoming streams. Communicates with plugin-goscript-cloudflare.ts.
type CloudflarePluginIo struct {
	// openStreamFunc is the JS function for the outgoing stream bridge.
	openStreamFunc js.Value
	// setAcceptStreamFunc is the JS function for the incoming stream bridge.
	setAcceptStreamFunc js.Value
}

// NewCloudflarePluginIo constructs the Cloudflare plugin i/o.
//
// openStreamToWebRuntime: see BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME
// setAcceptStream: see BLDR_PLUGIN_SET_ACCEPT_STREAM
func NewCloudflarePluginIo(openStreamToWebRuntime, setAcceptStream js.Value) (*CloudflarePluginIo, error) {
	if setAcceptStream.IsUndefined() || setAcceptStream.IsNull() || setAcceptStream.Type() != js.TypeFunction {
		return nil, errors.Errorf("js: %v is not a function", globalSetAcceptStream)
	}
	if openStreamToWebRuntime.IsUndefined() || openStreamToWebRuntime.IsNull() || openStreamToWebRuntime.Type() != js.TypeFunction {
		return nil, errors.Errorf("js: %v is not a function", globalOpenStreamToWebRuntime)
	}
	return &CloudflarePluginIo{
		openStreamFunc:      openStreamToWebRuntime,
		setAcceptStreamFunc: setAcceptStream,
	}, nil
}

// OpenStream opens an RPC stream via openStreamToWebRuntime.
func (p *CloudflarePluginIo) OpenStream(
	ctx context.Context,
	msgHandler srpc.PacketDataHandler,
	closeHandler srpc.CloseHandler,
) (srpc.PacketWriter, error) {
	return web_runtime_wasm.NewPushableOpenStream(p.openStreamFunc)(ctx, msgHandler, closeHandler)
}

var _ pluginTransport = ((*CloudflarePluginIo)(nil))

// SetAcceptStreams registers incoming streams until ctx is canceled.
func (p *CloudflarePluginIo) SetAcceptStreams(ctx context.Context, invoker srpc.Invoker) {
	var activeMtx sync.Mutex
	activeWriters := map[*deferredPacketWriter]struct{}{}
	closeActiveWriters := func() {
		activeMtx.Lock()
		writers := activeWriters
		activeWriters = map[*deferredPacketWriter]struct{}{}
		activeMtx.Unlock()
		for writer := range writers {
			_ = writer.Close()
		}
	}

	// Register one callback that gives each accepted stream its own lifetime.
	acceptStreamFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		defer recoverCloudflareJSCallback("accept stream", func(err error) {})
		if ctx.Err() != nil || len(args) == 0 || args[0].IsUndefined() || args[0].IsNull() || args[0].Type() != js.TypeFunction {
			return nil
		}

		writer := newDeferredPacketWriter()
		activeMtx.Lock()
		activeWriters[writer] = struct{}{}
		activeMtx.Unlock()
		remove := func() {
			activeMtx.Lock()
			delete(activeWriters, writer)
			activeMtx.Unlock()
		}
		serverRPC := srpc.NewServerRPC(ctx, invoker, writer)
		done := make(chan struct{})
		var doneMtx sync.Mutex
		finish := func() {
			doneMtx.Lock()
			select {
			case <-done:
			default:
				close(done)
			}
			doneMtx.Unlock()
			remove()
			_ = writer.Close()
		}
		packetWriter, err := web_runtime_wasm.NewPushableOpenStream(args[0])(
			ctx,
			serverRPC.HandlePacketData,
			func(err error) {
				serverRPC.HandleStreamClose(err)
				finish()
			},
		)
		if err != nil {
			finish()
			return nil
		}
		writer.setWriter(packetWriter)

		go func() {
			select {
			case <-ctx.Done():
				finish()
			case <-done:
			}
		}()
		return nil
	})
	p.setAcceptStreamFunc.Invoke(acceptStreamFn)
	go func() {
		<-ctx.Done()
		p.setAcceptStreamFunc.Invoke(js.Undefined())
		releaseCloudflareJSFunc(acceptStreamFn)
		closeActiveWriters()
	}()
}

// deferredPacketWriter buffers owned packets until the host writer resolves.
type deferredPacketWriter struct {
	mtx    sync.Mutex
	writer srpc.PacketWriter
	queue  []*srpc.Packet
	err    error
	closed bool
}

// newDeferredPacketWriter constructs an unresolved packet writer.
func newDeferredPacketWriter() *deferredPacketWriter { return &deferredPacketWriter{} }

// WritePacket writes now or retains an owned clone until resolution.
func (w *deferredPacketWriter) WritePacket(pkt *srpc.Packet) error {
	w.mtx.Lock()
	if w.err != nil {
		err := w.err
		w.mtx.Unlock()
		return err
	}
	if w.closed {
		w.mtx.Unlock()
		return io.ErrClosedPipe
	}
	if w.writer == nil {
		w.queue = append(w.queue, pkt.CloneVT())
		w.mtx.Unlock()
		return nil
	}
	writer := w.writer
	w.mtx.Unlock()
	return writer.WritePacket(pkt)
}

// setWriter resolves the writer and flushes all retained packets.
func (w *deferredPacketWriter) setWriter(writer srpc.PacketWriter) {
	w.mtx.Lock()
	if w.closed {
		w.mtx.Unlock()
		_ = writer.Close()
		return
	}
	w.writer = writer
	queue := w.queue
	w.queue = nil
	w.mtx.Unlock()

	for _, pkt := range queue {
		if err := writer.WritePacket(pkt); err != nil {
			w.mtx.Lock()
			w.err = err
			w.mtx.Unlock()
			_ = writer.Close()
			return
		}
	}
}

// Close closes the resolved writer and releases retained packets.
func (w *deferredPacketWriter) Close() error {
	w.mtx.Lock()
	if w.closed {
		err := w.err
		w.mtx.Unlock()
		return err
	}
	w.closed = true
	writer := w.writer
	w.queue = nil
	err := w.err
	w.mtx.Unlock()

	if writer != nil {
		if closeErr := writer.Close(); err == nil {
			err = closeErr
		}
	}
	return err
}

var _ srpc.PacketWriter = (*deferredPacketWriter)(nil)

func releaseCloudflareJSFunc(fn js.Func) {
	if runtime.Compiler != "tinygo" {
		fn.Release()
		return
	}
	time.AfterFunc(0, fn.Release)
}

func recoverCloudflareJSCallback(label string, onErr func(error)) {
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
