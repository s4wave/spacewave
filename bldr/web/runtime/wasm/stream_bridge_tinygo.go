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

	mtx    sync.Mutex
	events tinyGoPluginStreamEventOwner
}

type tinyGoPluginRuntimeOwner struct {
	mtx          sync.Mutex
	nextOpID     uint32
	openOps      map[uint32]*tinyGoPluginOpenOp
	streams      map[uint32]*tinyGoPluginStream
	acceptCtx    context.Context
	acceptInvoke srpc.Invoker
}

var tinyGoPluginRuntime = tinyGoPluginRuntimeOwner{}

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
	tinyGoPluginRuntime.setAcceptStreams(ctx, invoker)

	tinyGoPluginSetAcceptStreams(1)
	go func() {
		<-ctx.Done()
		streams := tinyGoPluginRuntime.closeAcceptStreams(ctx)

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
	tinyGoPluginRuntime.mtx.Lock()
	defer tinyGoPluginRuntime.mtx.Unlock()
	tinyGoPluginRuntime.ensureLocked()
	tinyGoPluginRuntime.nextOpID++
	if tinyGoPluginRuntime.nextOpID == 0 {
		tinyGoPluginRuntime.nextOpID++
	}
	op := &tinyGoPluginOpenOp{
		done:         make(chan struct{}),
		msgHandler:   msgHandler,
		closeHandler: closeHandler,
	}
	tinyGoPluginRuntime.openOps[tinyGoPluginRuntime.nextOpID] = op
	return tinyGoPluginRuntime.nextOpID, op
}

func (o *tinyGoPluginRuntimeOwner) ensureLocked() {
	if o.openOps == nil {
		o.openOps = map[uint32]*tinyGoPluginOpenOp{}
	}
	if o.streams == nil {
		o.streams = map[uint32]*tinyGoPluginStream{}
	}
}

func takeTinyGoPluginOpenOp(opID uint32) *tinyGoPluginOpenOp {
	return tinyGoPluginRuntime.takeOpenOp(opID)
}

func (o *tinyGoPluginRuntimeOwner) takeOpenOp(opID uint32) *tinyGoPluginOpenOp {
	o.mtx.Lock()
	op := o.openOps[opID]
	delete(o.openOps, opID)
	o.mtx.Unlock()
	return op
}

func (o *tinyGoPluginRuntimeOwner) setAcceptStreams(ctx context.Context, invoker srpc.Invoker) {
	o.mtx.Lock()
	o.ensureLocked()
	o.acceptCtx = ctx
	o.acceptInvoke = invoker
	o.mtx.Unlock()
}

func (o *tinyGoPluginRuntimeOwner) closeAcceptStreams(ctx context.Context) []*tinyGoPluginStream {
	o.mtx.Lock()
	if o.acceptCtx == ctx {
		o.acceptCtx = nil
		o.acceptInvoke = nil
	}
	streams := make([]*tinyGoPluginStream, 0, len(o.streams))
	for _, stream := range o.streams {
		streams = append(streams, stream)
	}
	o.mtx.Unlock()
	return streams
}

func (o *tinyGoPluginRuntimeOwner) acceptState() (context.Context, srpc.Invoker) {
	o.mtx.Lock()
	ctx := o.acceptCtx
	invoker := o.acceptInvoke
	o.mtx.Unlock()
	return ctx, invoker
}

func (o *tinyGoPluginRuntimeOwner) resolveOpen(opID, streamID uint32) bool {
	o.mtx.Lock()
	o.ensureLocked()
	op := o.openOps[opID]
	if op == nil || op.closed {
		o.mtx.Unlock()
		return false
	}
	op.closed = true
	stream := newTinyGoPluginStream(streamID, op.msgHandler, op.closeHandler)
	op.stream = stream
	o.streams[streamID] = stream
	close(op.done)
	o.mtx.Unlock()
	return true
}

func (o *tinyGoPluginRuntimeOwner) rejectOpen(opID, errBytesID, errLen uint32) bool {
	o.mtx.Lock()
	op := o.openOps[opID]
	if op == nil || op.closed {
		o.mtx.Unlock()
		return false
	}
	op.closed = true
	op.rejected = true
	op.errBytesID = errBytesID
	op.errLen = errLen
	close(op.done)
	o.mtx.Unlock()
	return true
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
		id: streamID,
	}
	stream.events.init()
	go stream.events.run(stream)
	return stream
}

func registerTinyGoPluginStream(stream *tinyGoPluginStream) {
	tinyGoPluginRuntime.registerStream(stream)
}

func (o *tinyGoPluginRuntimeOwner) registerStream(stream *tinyGoPluginStream) {
	o.mtx.Lock()
	o.ensureLocked()
	o.streams[stream.id] = stream
	o.mtx.Unlock()
}

func lookupTinyGoPluginStream(streamID uint32) *tinyGoPluginStream {
	return tinyGoPluginRuntime.lookupStream(streamID)
}

func (o *tinyGoPluginRuntimeOwner) lookupStream(streamID uint32) *tinyGoPluginStream {
	o.mtx.Lock()
	stream := o.streams[streamID]
	o.mtx.Unlock()
	return stream
}

func removeTinyGoPluginStream(stream *tinyGoPluginStream) {
	tinyGoPluginRuntime.removeStream(stream)
}

func (o *tinyGoPluginRuntimeOwner) removeStream(stream *tinyGoPluginStream) {
	o.mtx.Lock()
	if o.streams[stream.id] == stream {
		delete(o.streams, stream.id)
	}
	o.mtx.Unlock()
}

func (s *tinyGoPluginStream) WritePacket(pkt *srpc.Packet) error {
	data, err := pkt.MarshalVT()
	if err != nil {
		return err
	}
	return s.WritePacketData(data)
}

func (s *tinyGoPluginStream) WritePacketData(data []byte) error {
	if s.events.isClosed() {
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
	if !s.events.markClosed() {
		return nil
	}

	closed := tinyGoPluginStreamClose(s.id) != 0
	s.releaseJS()
	if !closed {
		return errors.New("tinygo plugin stream close failed")
	}
	return nil
}

func (s *tinyGoPluginStream) handleMessage(bytes []byte, handled func(error)) {
	if s.events.isClosed() {
		if handled != nil {
			handled(io.ErrClosedPipe)
		}
		return
	}
	s.mtx.Lock()
	incoming := s.incoming
	s.mtx.Unlock()
	if incoming == nil {
		if handled != nil {
			handled(io.ErrClosedPipe)
		}
		return
	}
	incoming.HandleWithResult(bytes, handled)
}

func (s *tinyGoPluginStream) handleClose(err error) {
	if !s.events.markClosed() {
		return
	}
	s.mtx.Lock()
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
		msg, hasMsg, closeEvent, hasClose := s.events.releasePending()

		dropTinyGoPluginStreamEvents(msg, hasMsg, closeEvent, hasClose)
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

type tinyGoPluginStreamEventOwner struct {
	mtx            sync.Mutex
	notify         chan struct{}
	done           chan struct{}
	closed         bool
	messagePending bool
	messageEvent   tinyGoPluginStreamMessageEvent
	closePending   bool
	closeEvent     tinyGoPluginStreamCloseEvent
}

type tinyGoPluginStreamEventKind uint8

const (
	tinyGoPluginStreamEventNone tinyGoPluginStreamEventKind = iota
	tinyGoPluginStreamEventMessage
	tinyGoPluginStreamEventClose
)

type tinyGoPluginStreamEvent struct {
	kind       tinyGoPluginStreamEventKind
	message    tinyGoPluginStreamMessageEvent
	closeEvent tinyGoPluginStreamCloseEvent
}

func (e *tinyGoPluginStreamEventOwner) init() {
	e.notify = make(chan struct{}, 1)
	e.done = make(chan struct{})
}

func (e *tinyGoPluginStreamEventOwner) isClosed() bool {
	e.mtx.Lock()
	closed := e.closed
	e.mtx.Unlock()
	return closed
}

func (e *tinyGoPluginStreamEventOwner) markClosed() bool {
	e.mtx.Lock()
	if e.closed {
		e.mtx.Unlock()
		return false
	}
	e.closed = true
	e.mtx.Unlock()
	return true
}

func (e *tinyGoPluginStreamEventOwner) enqueueMessage(bytesID, length uint32) bool {
	e.mtx.Lock()
	if e.closed || e.messagePending {
		e.mtx.Unlock()
		return false
	}
	e.messagePending = true
	e.messageEvent = tinyGoPluginStreamMessageEvent{
		bytesID: bytesID,
		length:  length,
	}
	e.signalLocked()
	e.mtx.Unlock()
	return true
}

func (e *tinyGoPluginStreamEventOwner) enqueueClose(errBytesID, errLen uint32) bool {
	e.mtx.Lock()
	if e.closed || e.closePending {
		e.mtx.Unlock()
		return false
	}
	e.closePending = true
	e.closeEvent = tinyGoPluginStreamCloseEvent{
		errBytesID: errBytesID,
		errLen:     errLen,
	}
	e.signalLocked()
	e.mtx.Unlock()
	return true
}

func (e *tinyGoPluginStreamEventOwner) signalLocked() {
	select {
	case e.notify <- struct{}{}:
	default:
	}
}

func (e *tinyGoPluginStreamEventOwner) run(stream *tinyGoPluginStream) {
	for {
		event := e.next()
		switch event.kind {
		case tinyGoPluginStreamEventMessage:
			stream.handleMessageEvent(event.message)
			continue
		case tinyGoPluginStreamEventClose:
			stream.handleClose(tinyGoPluginError(event.closeEvent.errBytesID, event.closeEvent.errLen))
			continue
		default:
			return
		}
	}
}

func (e *tinyGoPluginStreamEventOwner) next() tinyGoPluginStreamEvent {
	e.mtx.Lock()
	for !e.messagePending && !e.closePending && !e.closed {
		e.mtx.Unlock()
		select {
		case <-e.notify:
		case <-e.done:
			return tinyGoPluginStreamEvent{}
		}
		e.mtx.Lock()
	}
	if e.messagePending {
		event := e.messageEvent
		e.messageEvent = tinyGoPluginStreamMessageEvent{}
		e.messagePending = false
		e.mtx.Unlock()
		return tinyGoPluginStreamEvent{
			kind:    tinyGoPluginStreamEventMessage,
			message: event,
		}
	}
	if e.closePending {
		event := e.closeEvent
		e.closeEvent = tinyGoPluginStreamCloseEvent{}
		e.closePending = false
		e.mtx.Unlock()
		return tinyGoPluginStreamEvent{
			kind:       tinyGoPluginStreamEventClose,
			closeEvent: event,
		}
	}
	e.mtx.Unlock()
	return tinyGoPluginStreamEvent{}
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

func (e *tinyGoPluginStreamEventOwner) releasePending() (
	tinyGoPluginStreamMessageEvent,
	bool,
	tinyGoPluginStreamCloseEvent,
	bool,
) {
	e.mtx.Lock()
	e.closed = true
	msg := e.messageEvent
	hasMsg := e.messagePending
	closeEvent := e.closeEvent
	hasClose := e.closePending
	if hasMsg {
		e.messageEvent = tinyGoPluginStreamMessageEvent{}
		e.messagePending = false
	}
	if hasClose {
		e.closeEvent = tinyGoPluginStreamCloseEvent{}
		e.closePending = false
	}
	close(e.done)
	e.mtx.Unlock()
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
	if !tinyGoPluginRuntime.resolveOpen(opID, streamID) {
		tinyGoPluginStreamClose(streamID)
		tinyGoPluginStreamRelease(streamID)
		return
	}
}

//export BLDR_PLUGIN_STREAM_OPEN_REJECT
func tinyGoPluginStreamOpenReject(opID uint32, errBytesID uint32, errLen uint32) {
	if !tinyGoPluginRuntime.rejectOpen(opID, errBytesID, errLen) {
		dropTinyGoPluginBytes(errBytesID, errLen)
		return
	}
}

//export BLDR_PLUGIN_STREAM_MESSAGE
func tinyGoPluginStreamMessage(streamID uint32, bytesID uint32, length uint32) {
	stream := lookupTinyGoPluginStream(streamID)
	if stream == nil || !stream.events.enqueueMessage(bytesID, length) {
		dropTinyGoPluginBytes(bytesID, length)
		tinyGoPluginStreamMessageHandled(bytesID, 0)
		return
	}
}

//export BLDR_PLUGIN_STREAM_CLOSE
func tinyGoPluginStreamClosed(streamID uint32, errBytesID uint32, errLen uint32) {
	stream := lookupTinyGoPluginStream(streamID)
	if stream == nil || !stream.events.enqueueClose(errBytesID, errLen) {
		dropTinyGoPluginBytes(errBytesID, errLen)
		return
	}
}

//export BLDR_PLUGIN_STREAM_ACCEPT
func tinyGoPluginStreamAccept(streamID uint32) {
	ctx, invoker := tinyGoPluginRuntime.acceptState()
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
