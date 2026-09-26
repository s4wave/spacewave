// Package signaling_rpc_frame carries srpc streams over one WebSocket with one
// Frame per binary message.
//
// A stream multiplexer keeps windows and buffers on both ends, so its server
// must stay resident for the life of the socket. Frames are independent: a
// server can decode each message on its own, which lets a Cloudflare Durable
// Object hibernate between signaling messages.
package signaling_rpc_frame

import (
	"context"
	"sync"

	websocket "github.com/aperturerobotics/go-websocket"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
)

// ReadLimit is the largest frame either side accepts.
const ReadLimit = 256 * 1024

// ErrTextFrame is returned when the remote sends a text message.
var ErrTextFrame = errors.New("signaling websocket expects binary frames")

// AcceptFunc attaches a stream the remote opened. It returns the handlers
// for the stream's packets and its close.
type AcceptFunc func(w srpc.PacketWriter) (srpc.PacketDataHandler, srpc.CloseHandler)

// Conn carries srpc streams over one WebSocket.
type Conn struct {
	ctx context.Context
	ws  *websocket.Conn

	mtx     sync.Mutex
	nextID  uint32
	streams map[uint32]*stream
	err     error
}

// stream is one srpc stream on the socket.
type stream struct {
	handler      srpc.PacketDataHandler
	closeHandler srpc.CloseHandler
	// localClosed and remoteClosed record each side's close frame. The entry
	// is removed when both are set.
	localClosed  bool
	remoteClosed bool
}

// NewConn wraps a WebSocket. Writes use ctx; ReadPump must run for streams
// to receive packets.
func NewConn(ctx context.Context, ws *websocket.Conn) *Conn {
	ws.SetReadLimit(ReadLimit)
	return &Conn{ctx: ctx, ws: ws, streams: make(map[uint32]*stream)}
}

// OpenStream opens a stream to the remote. It implements srpc.OpenStreamFunc.
func (c *Conn) OpenStream(
	_ context.Context,
	msgHandler srpc.PacketDataHandler,
	closeHandler srpc.CloseHandler,
) (srpc.PacketWriter, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.err != nil {
		return nil, c.err
	}
	c.nextID++
	id := c.nextID
	c.streams[id] = &stream{handler: msgHandler, closeHandler: closeHandler}
	return &writer{c: c, id: id}, nil
}

// ReadPump dispatches frames to their streams until the socket fails, then
// closes every stream with the error and returns it. accept attaches streams
// the remote opens; with a nil accept, frames for unknown streams are dropped.
func (c *Conn) ReadPump(accept AcceptFunc) error {
	err := c.readPump(accept)

	// Fail every open stream and refuse new ones.
	c.mtx.Lock()
	c.err = err
	streams := c.streams
	c.streams = nil
	c.mtx.Unlock()
	for _, s := range streams {
		if !s.remoteClosed {
			s.closeHandler(err)
		}
	}
	return err
}

func (c *Conn) readPump(accept AcceptFunc) error {
	for {
		typ, data, err := c.ws.Read(c.ctx)
		if err != nil {
			return err
		}
		if typ != websocket.MessageBinary {
			return ErrTextFrame
		}
		frame := &Frame{}
		if err := frame.UnmarshalVT(data); err != nil {
			return errors.Wrap(err, "decode signaling frame")
		}
		c.handleFrame(frame, accept)
	}
}

// handleFrame delivers one frame. A packet the stream rejects closes the
// stream from both sides.
func (c *Conn) handleFrame(frame *Frame, accept AcceptFunc) {
	// Only the read pump sets handlers, so accept runs outside the lock.
	id := frame.GetStreamId()
	c.mtx.Lock()
	s := c.streams[id]
	opened := s == nil && accept != nil && !frame.GetClose()
	if opened {
		s = &stream{}
		c.streams[id] = s
	}
	c.mtx.Unlock()
	if opened {
		s.handler, s.closeHandler = accept(&writer{c: c, id: id})
	}
	if s == nil || s.remoteClosed {
		return
	}

	if packet := frame.GetPacket(); len(packet) != 0 {
		if err := s.handler(packet); err != nil {
			c.remoteClose(id, s, err)
			_ = c.closeStream(id)
			return
		}
	}
	if frame.GetClose() {
		c.remoteClose(id, s, nil)
	}
}

// remoteClose ends the remote side of a stream and reports it once.
func (c *Conn) remoteClose(id uint32, s *stream, err error) {
	c.mtx.Lock()
	if s.remoteClosed {
		c.mtx.Unlock()
		return
	}
	s.remoteClosed = true
	if s.localClosed {
		delete(c.streams, id)
	}
	c.mtx.Unlock()
	s.closeHandler(err)
}

// writeFrame sends one frame.
func (c *Conn) writeFrame(frame *Frame) error {
	data, err := frame.MarshalVT()
	if err != nil {
		return err
	}
	return c.ws.Write(c.ctx, websocket.MessageBinary, data)
}

// closeStream sends the local close frame once.
func (c *Conn) closeStream(id uint32) error {
	c.mtx.Lock()
	s := c.streams[id]
	if s == nil || s.localClosed {
		c.mtx.Unlock()
		return nil
	}
	s.localClosed = true
	if s.remoteClosed {
		delete(c.streams, id)
	}
	c.mtx.Unlock()
	return c.writeFrame(&Frame{StreamId: id, Close: true})
}

// writer writes one stream's packets.
type writer struct {
	c  *Conn
	id uint32
}

// WritePacket sends a packet on the stream.
func (w *writer) WritePacket(p *srpc.Packet) error {
	data, err := p.MarshalVT()
	if err != nil {
		return err
	}
	return w.c.writeFrame(&Frame{StreamId: w.id, Packet: data})
}

// Close ends the local side of the stream.
func (w *writer) Close() error {
	return w.c.closeStream(w.id)
}

// NewServerAccept returns an AcceptFunc that runs each stream as an srpc call
// on invoker.
func NewServerAccept(ctx context.Context, invoker srpc.Invoker) AcceptFunc {
	return func(w srpc.PacketWriter) (srpc.PacketDataHandler, srpc.CloseHandler) {
		rpc := srpc.NewServerRPC(ctx, invoker, w)
		return rpc.HandlePacketData, rpc.HandleStreamClose
	}
}

// _ is a type assertion
var _ srpc.PacketWriter = (*writer)(nil)
