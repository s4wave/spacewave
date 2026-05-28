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
	rejected     bool
	errBytesID   uint32
	errLen       uint32
	closed       bool
}

type tinyGoPluginStream struct {
	id       uint32
	incoming *serialPacketDataHandler
	release  sync.Once
	notify   chan struct{}
	done     chan struct{}

	mtx            sync.Mutex
	closed         bool
	messagePending bool
	messageEvent   tinyGoPluginStreamMessageEvent
	closePending   bool
	closeEvent     tinyGoPluginStreamCloseEvent
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

//go:wasmimport gojs bldr.plugin.streamDropBytes
func tinyGoPluginStreamDropBytes(bytesID uint32) uint32

//go:wasmimport gojs bldr.plugin.streamMessageHandled
func tinyGoPluginStreamMessageHandled(bytesID uint32, delivered uint32)

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
		return finishTinyGoPluginOpenOp(opID, op)
	case <-ctx.Done():
		select {
		case <-op.done:
			return finishTinyGoPluginOpenOp(opID, op)
		default:
		}
		cancelTinyGoPluginOpenOp(opID)
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

func takeTinyGoPluginOpenOp(opID uint32) *tinyGoPluginOpenOp {
	tinyGoPluginMu.Lock()
	op := tinyGoPluginOpenOps[opID]
	delete(tinyGoPluginOpenOps, opID)
	tinyGoPluginMu.Unlock()
	return op
}

func finishTinyGoPluginOpenOp(opID uint32, op *tinyGoPluginOpenOp) (srpc.PacketWriter, error) {
	if taken := takeTinyGoPluginOpenOp(opID); taken != nil {
		op = taken
	}
	if op.rejected {
		err := tinyGoPluginError(op.errBytesID, op.errLen)
		op.errBytesID = 0
		op.errLen = 0
		if err == nil {
			err = errors.New("open stream rejected")
		}
		return nil, err
	}
	return op.stream, nil
}

func cancelTinyGoPluginOpenOp(opID uint32) {
	op := takeTinyGoPluginOpenOp(opID)
	if op == nil {
		return
	}
	if op.rejected {
		dropTinyGoPluginBytes(op.errBytesID, op.errLen)
	}
	if op.stream != nil {
		_ = op.stream.Close()
	}
}

func newTinyGoPluginStream(
	streamID uint32,
	msgHandler srpc.PacketDataHandler,
	closeHandler srpc.CloseHandler,
) *tinyGoPluginStream {
	stream := newTinyGoPluginStreamState(streamID)
	stream.incoming = newSerialPacketDataHandler(msgHandler, closeHandler, stream.releaseJS)
	return stream
}

func newTinyGoPluginStreamState(streamID uint32) *tinyGoPluginStream {
	stream := &tinyGoPluginStream{
		id:     streamID,
		notify: make(chan struct{}, 1),
		done:   make(chan struct{}),
	}
	go stream.runExportEvents()
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

func (s *tinyGoPluginStream) handleMessage(bytes []byte, handled func(error)) {
	s.mtx.Lock()
	closed := s.closed
	incoming := s.incoming
	s.mtx.Unlock()
	if closed || incoming == nil {
		if handled != nil {
			handled(io.ErrClosedPipe)
		}
		return
	}
	incoming.HandleWithResult(bytes, handled)
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
		s.mtx.Lock()
		s.closed = true
		msg, hasMsg, closeEvent, hasClose := s.takePendingEventsLocked()
		s.mtx.Unlock()

		dropTinyGoPluginStreamEvents(msg, hasMsg, closeEvent, hasClose)
		close(s.done)
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

func dropTinyGoPluginBytes(bytesID, _ uint32) {
	if bytesID == 0 {
		return
	}
	tinyGoPluginStreamDropBytes(bytesID)
}

type tinyGoPluginStreamMessageEvent struct {
	bytesID uint32
	length  uint32
}

type tinyGoPluginStreamCloseEvent struct {
	errBytesID uint32
	errLen     uint32
}

func (s *tinyGoPluginStream) enqueueMessage(bytesID, length uint32) bool {
	s.mtx.Lock()
	if s.closed || s.messagePending {
		s.mtx.Unlock()
		return false
	}
	s.messagePending = true
	s.messageEvent = tinyGoPluginStreamMessageEvent{
		bytesID: bytesID,
		length:  length,
	}
	s.signalLocked()
	s.mtx.Unlock()
	return true
}

func (s *tinyGoPluginStream) enqueueClose(errBytesID, errLen uint32) bool {
	s.mtx.Lock()
	if s.closed || s.closePending {
		s.mtx.Unlock()
		return false
	}
	s.closePending = true
	s.closeEvent = tinyGoPluginStreamCloseEvent{
		errBytesID: errBytesID,
		errLen:     errLen,
	}
	s.signalLocked()
	s.mtx.Unlock()
	return true
}

func (s *tinyGoPluginStream) signalLocked() {
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *tinyGoPluginStream) runExportEvents() {
	for {
		s.mtx.Lock()
		for !s.messagePending && !s.closePending && !s.closed {
			s.mtx.Unlock()
			select {
			case <-s.notify:
			case <-s.done:
				return
			}
			s.mtx.Lock()
		}
		if s.messagePending {
			event := s.messageEvent
			s.messageEvent = tinyGoPluginStreamMessageEvent{}
			s.messagePending = false
			s.mtx.Unlock()
			s.handleMessageEvent(event)
			continue
		}
		if s.closePending {
			event := s.closeEvent
			s.closeEvent = tinyGoPluginStreamCloseEvent{}
			s.closePending = false
			s.mtx.Unlock()
			s.handleClose(tinyGoPluginError(event.errBytesID, event.errLen))
			continue
		}
		s.mtx.Unlock()
		return
	}
}

func (s *tinyGoPluginStream) handleMessageEvent(event tinyGoPluginStreamMessageEvent) {
	bytes, ok := copyTinyGoPluginBytes(event.bytesID, event.length)
	if !ok {
		tinyGoPluginStreamMessageHandled(event.bytesID, 0)
		s.handleClose(errors.New("tinygo plugin stream bytes unavailable"))
		return
	}
	s.handleMessage(bytes, func(err error) {
		if err != nil {
			tinyGoPluginStreamMessageHandled(event.bytesID, 0)
			return
		}
		tinyGoPluginStreamMessageHandled(event.bytesID, 1)
	})
}

func (s *tinyGoPluginStream) takePendingEventsLocked() (
	tinyGoPluginStreamMessageEvent,
	bool,
	tinyGoPluginStreamCloseEvent,
	bool,
) {
	msg := s.messageEvent
	hasMsg := s.messagePending
	closeEvent := s.closeEvent
	hasClose := s.closePending
	if hasMsg {
		s.messageEvent = tinyGoPluginStreamMessageEvent{}
		s.messagePending = false
	}
	if hasClose {
		s.closeEvent = tinyGoPluginStreamCloseEvent{}
		s.closePending = false
	}
	return msg, hasMsg, closeEvent, hasClose
}

func dropTinyGoPluginStreamEvents(
	msg tinyGoPluginStreamMessageEvent,
	hasMsg bool,
	closeEvent tinyGoPluginStreamCloseEvent,
	hasClose bool,
) {
	if hasMsg {
		dropTinyGoPluginBytes(msg.bytesID, msg.length)
		tinyGoPluginStreamMessageHandled(msg.bytesID, 0)
	}
	if hasClose {
		dropTinyGoPluginBytes(closeEvent.errBytesID, closeEvent.errLen)
	}
}

// Exported TinyGo plugin callbacks are synchronous JavaScript completion hooks.
// Use direct //export callbacks so TinyGo asyncify does not allocate a fresh
// goroutine stack before each callback body.
//
//export BLDR_PLUGIN_STREAM_OPEN_RESOLVE
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

//export BLDR_PLUGIN_STREAM_OPEN_REJECT
func tinyGoPluginStreamOpenReject(opID uint32, errBytesID uint32, errLen uint32) {
	tinyGoPluginMu.Lock()
	op := tinyGoPluginOpenOps[opID]
	if op == nil || op.closed {
		tinyGoPluginMu.Unlock()
		dropTinyGoPluginBytes(errBytesID, errLen)
		return
	}
	op.closed = true
	op.rejected = true
	op.errBytesID = errBytesID
	op.errLen = errLen
	close(op.done)
	tinyGoPluginMu.Unlock()
}

//export BLDR_PLUGIN_STREAM_MESSAGE
func tinyGoPluginStreamMessage(streamID uint32, bytesID uint32, length uint32) {
	stream := lookupTinyGoPluginStream(streamID)
	if stream == nil || !stream.enqueueMessage(bytesID, length) {
		dropTinyGoPluginBytes(bytesID, length)
		tinyGoPluginStreamMessageHandled(bytesID, 0)
		return
	}
}

//export BLDR_PLUGIN_STREAM_CLOSE
func tinyGoPluginStreamClosed(streamID uint32, errBytesID uint32, errLen uint32) {
	stream := lookupTinyGoPluginStream(streamID)
	if stream == nil || !stream.enqueueClose(errBytesID, errLen) {
		dropTinyGoPluginBytes(errBytesID, errLen)
		return
	}
}

//export BLDR_PLUGIN_STREAM_ACCEPT
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

	stream := newTinyGoPluginStreamState(streamID)
	serverRPC := srpc.NewServerRPC(ctx, invoker, stream)
	stream.incoming = newSerialPacketDataHandler(
		serverRPC.HandlePacketData,
		serverRPC.HandleStreamClose,
		stream.releaseJS,
	)
	registerTinyGoPluginStream(stream)
}

var _ srpc.PacketWriter = (*tinyGoPluginStream)(nil)
