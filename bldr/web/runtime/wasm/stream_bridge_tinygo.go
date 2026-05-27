//go:build js && tinygo

package web_runtime_wasm

import (
	"context"
	"io"
	"sync"
	"unsafe"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
)

type tinyGoPluginOpenOp struct {
	done         chan struct{}
	msgHandler   srpc.PacketDataHandler
	closeHandler srpc.CloseHandler
	stream       *tinyGoPluginStream
	err          error
	closed       bool
}

type tinyGoPluginStream struct {
	id       uint32
	incoming *serialPacketDataHandler
	release  sync.Once

	mtx    sync.Mutex
	closed bool
}

var (
	tinyGoPluginMu           sync.Mutex
	tinyGoPluginNextOpID     uint32
	tinyGoPluginOpenOps      = map[uint32]*tinyGoPluginOpenOp{}
	tinyGoPluginStreams      = map[uint32]*tinyGoPluginStream{}
	tinyGoPluginAcceptCtx    context.Context
	tinyGoPluginAcceptInvoke srpc.Invoker
)

//go:wasmimport gojs bldr.plugin.openStream
func tinyGoPluginOpenStream(opID uint32)

//go:wasmimport gojs bldr.plugin.streamWrite
func tinyGoPluginStreamWrite(streamID uint32, ptr unsafe.Pointer, len uint32) uint32

//go:wasmimport gojs bldr.plugin.streamClose
func tinyGoPluginStreamClose(streamID uint32) uint32

//go:wasmimport gojs bldr.plugin.streamRelease
func tinyGoPluginStreamRelease(streamID uint32)

//go:wasmimport gojs bldr.plugin.streamTakeBytes
func tinyGoPluginStreamTakeBytes(bytesID uint32, ptr unsafe.Pointer, len uint32) uint32

//go:wasmimport gojs bldr.plugin.setAcceptStreams
func tinyGoPluginSetAcceptStreams(enabled uint32)

func newTinyGoPluginOpenStream(
	ctx context.Context,
	msgHandler srpc.PacketDataHandler,
	closeHandler srpc.CloseHandler,
) (srpc.PacketWriter, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	opID, op := registerTinyGoPluginOpenOp(msgHandler, closeHandler)
	tinyGoPluginOpenStream(opID)

	select {
	case <-op.done:
		deleteTinyGoPluginOpenOp(opID)
		if op.err != nil {
			return nil, op.err
		}
		return op.stream, nil
	case <-ctx.Done():
		deleteTinyGoPluginOpenOp(opID)
		return nil, ctx.Err()
	}
}

func setTinyGoPluginAcceptStreams(ctx context.Context, invoker srpc.Invoker) {
	tinyGoPluginMu.Lock()
	tinyGoPluginAcceptCtx = ctx
	tinyGoPluginAcceptInvoke = invoker
	tinyGoPluginMu.Unlock()

	tinyGoPluginSetAcceptStreams(1)
	go func() {
		<-ctx.Done()
		tinyGoPluginMu.Lock()
		if tinyGoPluginAcceptCtx == ctx {
			tinyGoPluginAcceptCtx = nil
			tinyGoPluginAcceptInvoke = nil
		}
		streams := make([]*tinyGoPluginStream, 0, len(tinyGoPluginStreams))
		for _, stream := range tinyGoPluginStreams {
			streams = append(streams, stream)
		}
		tinyGoPluginMu.Unlock()

		tinyGoPluginSetAcceptStreams(0)
		for _, stream := range streams {
			_ = stream.Close()
		}
	}()
}

func registerTinyGoPluginOpenOp(
	msgHandler srpc.PacketDataHandler,
	closeHandler srpc.CloseHandler,
) (uint32, *tinyGoPluginOpenOp) {
	tinyGoPluginMu.Lock()
	defer tinyGoPluginMu.Unlock()
	tinyGoPluginNextOpID++
	if tinyGoPluginNextOpID == 0 {
		tinyGoPluginNextOpID++
	}
	op := &tinyGoPluginOpenOp{
		done:         make(chan struct{}),
		msgHandler:   msgHandler,
		closeHandler: closeHandler,
	}
	tinyGoPluginOpenOps[tinyGoPluginNextOpID] = op
	return tinyGoPluginNextOpID, op
}

func deleteTinyGoPluginOpenOp(opID uint32) {
	tinyGoPluginMu.Lock()
	delete(tinyGoPluginOpenOps, opID)
	tinyGoPluginMu.Unlock()
}

func newTinyGoPluginStream(
	streamID uint32,
	msgHandler srpc.PacketDataHandler,
	closeHandler srpc.CloseHandler,
) *tinyGoPluginStream {
	stream := &tinyGoPluginStream{id: streamID}
	stream.incoming = newSerialPacketDataHandler(msgHandler, closeHandler, stream.releaseJS)
	return stream
}

func registerTinyGoPluginStream(stream *tinyGoPluginStream) {
	tinyGoPluginMu.Lock()
	tinyGoPluginStreams[stream.id] = stream
	tinyGoPluginMu.Unlock()
}

func lookupTinyGoPluginStream(streamID uint32) *tinyGoPluginStream {
	tinyGoPluginMu.Lock()
	stream := tinyGoPluginStreams[streamID]
	tinyGoPluginMu.Unlock()
	return stream
}

func removeTinyGoPluginStream(stream *tinyGoPluginStream) {
	tinyGoPluginMu.Lock()
	if tinyGoPluginStreams[stream.id] == stream {
		delete(tinyGoPluginStreams, stream.id)
	}
	tinyGoPluginMu.Unlock()
}

func (s *tinyGoPluginStream) WritePacket(pkt *srpc.Packet) error {
	data, err := pkt.MarshalVT()
	if err != nil {
		return err
	}
	return s.WritePacketData(data)
}

func (s *tinyGoPluginStream) WritePacketData(data []byte) error {
	s.mtx.Lock()
	closed := s.closed
	s.mtx.Unlock()
	if closed {
		return io.ErrClosedPipe
	}

	var ptr unsafe.Pointer
	if len(data) != 0 {
		ptr = unsafe.Pointer(&data[0])
	}
	if tinyGoPluginStreamWrite(s.id, ptr, uint32(len(data))) == 0 {
		return errors.New("tinygo plugin stream write failed")
	}
	return nil
}

func (s *tinyGoPluginStream) Close() error {
	s.mtx.Lock()
	if s.closed {
		s.mtx.Unlock()
		return nil
	}
	s.closed = true
	s.mtx.Unlock()

	closed := tinyGoPluginStreamClose(s.id) != 0
	s.releaseJS()
	if !closed {
		return errors.New("tinygo plugin stream close failed")
	}
	return nil
}

func (s *tinyGoPluginStream) handleMessage(bytes []byte) {
	s.mtx.Lock()
	closed := s.closed
	incoming := s.incoming
	s.mtx.Unlock()
	if closed || incoming == nil {
		return
	}
	incoming.Handle(bytes)
}

func (s *tinyGoPluginStream) handleClose(err error) {
	s.mtx.Lock()
	if s.closed {
		s.mtx.Unlock()
		return
	}
	s.closed = true
	incoming := s.incoming
	s.mtx.Unlock()

	if incoming == nil {
		s.releaseJS()
		return
	}
	incoming.Close(err)
}

func (s *tinyGoPluginStream) releaseJS() {
	s.release.Do(func() {
		removeTinyGoPluginStream(s)
		tinyGoPluginStreamRelease(s.id)
	})
}

func copyTinyGoPluginBytes(bytesID, length uint32) ([]byte, bool) {
	if length == 0 {
		return nil, tinyGoPluginStreamTakeBytes(bytesID, nil, 0) != 0
	}
	bytes := make([]byte, int(length))
	if tinyGoPluginStreamTakeBytes(bytesID, unsafe.Pointer(&bytes[0]), length) == 0 {
		return nil, false
	}
	return bytes, true
}

func tinyGoPluginError(bytesID, length uint32) error {
	if bytesID == 0 && length == 0 {
		return nil
	}
	bytes, ok := copyTinyGoPluginBytes(bytesID, length)
	if !ok {
		return errors.New("tinygo plugin stream error unavailable")
	}
	return errors.New(string(bytes))
}

//go:wasmexport BLDR_PLUGIN_STREAM_OPEN_RESOLVE
func tinyGoPluginStreamOpenResolve(opID uint32, streamID uint32) {
	tinyGoPluginMu.Lock()
	op := tinyGoPluginOpenOps[opID]
	if op == nil || op.closed {
		tinyGoPluginMu.Unlock()
		tinyGoPluginStreamClose(streamID)
		tinyGoPluginStreamRelease(streamID)
		return
	}
	op.closed = true
	stream := newTinyGoPluginStream(streamID, op.msgHandler, op.closeHandler)
	op.stream = stream
	tinyGoPluginStreams[streamID] = stream
	close(op.done)
	tinyGoPluginMu.Unlock()
}

//go:wasmexport BLDR_PLUGIN_STREAM_OPEN_REJECT
func tinyGoPluginStreamOpenReject(opID uint32, errBytesID uint32, errLen uint32) {
	err := tinyGoPluginError(errBytesID, errLen)
	if err == nil {
		err = errors.New("open stream rejected")
	}

	tinyGoPluginMu.Lock()
	op := tinyGoPluginOpenOps[opID]
	if op == nil || op.closed {
		tinyGoPluginMu.Unlock()
		return
	}
	op.closed = true
	op.err = err
	close(op.done)
	tinyGoPluginMu.Unlock()
}

//go:wasmexport BLDR_PLUGIN_STREAM_MESSAGE
func tinyGoPluginStreamMessage(streamID uint32, bytesID uint32, length uint32) {
	bytes, ok := copyTinyGoPluginBytes(bytesID, length)
	if !ok {
		stream := lookupTinyGoPluginStream(streamID)
		if stream != nil {
			stream.handleClose(errors.New("tinygo plugin stream bytes unavailable"))
		}
		return
	}
	stream := lookupTinyGoPluginStream(streamID)
	if stream == nil {
		return
	}
	stream.handleMessage(bytes)
}

//go:wasmexport BLDR_PLUGIN_STREAM_CLOSE
func tinyGoPluginStreamClosed(streamID uint32, errBytesID uint32, errLen uint32) {
	stream := lookupTinyGoPluginStream(streamID)
	if stream == nil {
		if errBytesID != 0 || errLen != 0 {
			_, _ = copyTinyGoPluginBytes(errBytesID, errLen)
		}
		return
	}
	stream.handleClose(tinyGoPluginError(errBytesID, errLen))
}

//go:wasmexport BLDR_PLUGIN_STREAM_ACCEPT
func tinyGoPluginStreamAccept(streamID uint32) {
	tinyGoPluginMu.Lock()
	ctx := tinyGoPluginAcceptCtx
	invoker := tinyGoPluginAcceptInvoke
	tinyGoPluginMu.Unlock()
	if ctx == nil || invoker == nil || ctx.Err() != nil {
		tinyGoPluginStreamClose(streamID)
		tinyGoPluginStreamRelease(streamID)
		return
	}

	stream := &tinyGoPluginStream{id: streamID}
	serverRPC := srpc.NewServerRPC(ctx, invoker, stream)
	stream.incoming = newSerialPacketDataHandler(
		serverRPC.HandlePacketData,
		serverRPC.HandleStreamClose,
		stream.releaseJS,
	)
	registerTinyGoPluginStream(stream)
}

var _ srpc.PacketWriter = (*tinyGoPluginStream)(nil)
