package plan9fs

import (
	"context"
	"encoding/binary"

	"github.com/aperturerobotics/hydra/unixfs"
	"github.com/pkg/errors"
)

// Transport abstracts the 9p message transport.
type Transport interface {
	// ReadMessage reads the next 9p message from the transport.
	ReadMessage(ctx context.Context) ([]byte, error)
	// WriteMessage writes a 9p message to the transport.
	WriteMessage(ctx context.Context, data []byte) error
}

// Server implements a 9p2000.L filesystem server.
type Server struct {
	root  *unixfs.FSHandle
	msize uint32
	fids  *FidTable
}

// NewServer creates a new 9p server with the given root FSHandle.
func NewServer(root *unixfs.FSHandle) *Server {
	return &Server{
		root:  root,
		msize: defaultMsize,
		fids:  NewFidTable(),
	}
}

// Serve processes 9p messages from a transport until context cancellation or error.
func (s *Server) Serve(ctx context.Context, t Transport) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		msg, err := t.ReadMessage(ctx)
		if err != nil {
			return errors.Wrap(err, "read 9p message")
		}
		resp, err := s.HandleMessage(ctx, msg)
		if err != nil {
			return errors.Wrap(err, "handle 9p message")
		}
		if resp == nil {
			continue
		}
		if err := t.WriteMessage(ctx, resp); err != nil {
			return errors.Wrap(err, "write 9p response")
		}
	}
}

// HandleMessage processes a single 9p request and returns the response.
// Concurrent calls on different fids are safe. Concurrent calls on the
// same fid require external serialization.
func (s *Server) HandleMessage(ctx context.Context, msg []byte) ([]byte, error) {
	if len(msg) < headerSize {
		return nil, errors.New("message too short")
	}
	size := binary.LittleEndian.Uint32(msg[0:4])
	if int(size) != len(msg) {
		return nil, errors.Errorf("message size mismatch: header=%d actual=%d", size, len(msg))
	}
	msgType := msg[4]
	tag := binary.LittleEndian.Uint16(msg[5:7])

	// reject messages exceeding negotiated msize (TVERSION is exempt)
	if msgType != TVERSION && size > s.msize {
		return buildErrorResponse(tag, EIO), nil
	}
	payload := msg[headerSize:]

	resp, err := s.dispatch(ctx, msgType, tag, payload)
	if err != nil {
		return buildErrorResponse(tag, toErrno(err)), nil
	}
	return resp, nil
}

// dispatch routes a 9p message to the appropriate handler.
func (s *Server) dispatch(ctx context.Context, msgType uint8, tag uint16, payload []byte) ([]byte, error) {
	switch msgType {
	case TVERSION:
		return s.handleVersion(tag, payload)
	case TATTACH:
		return s.handleAttach(ctx, tag, payload)
	case TWALK:
		return s.handleWalk(ctx, tag, payload)
	case TLOPEN:
		return s.handleLopen(ctx, tag, payload)
	case TLCREATE:
		return s.handleLcreate(ctx, tag, payload)
	case TREAD:
		return s.handleRead(ctx, tag, payload)
	case TWRITE:
		return s.handleWrite(ctx, tag, payload)
	case TCLUNK:
		return s.handleClunk(tag, payload)
	case TREMOVE:
		return s.handleRemove(ctx, tag, payload)
	case TGETATTR:
		return s.handleGetattr(ctx, tag, payload)
	case TSETATTR:
		return s.handleSetattr(ctx, tag, payload)
	case TREADDIR:
		return s.handleReaddir(ctx, tag, payload)
	case TMKDIR:
		return s.handleMkdir(ctx, tag, payload)
	case TSYMLINK:
		return s.handleSymlink(ctx, tag, payload)
	case TREADLINK:
		return s.handleReadlink(ctx, tag, payload)
	case TUNLINKAT:
		return s.handleUnlinkat(ctx, tag, payload)
	case TRENAMEAT:
		return s.handleRenameat(ctx, tag, payload)
	case TMKNOD:
		return s.handleMknod(ctx, tag, payload)
	case TLINK:
		return s.handleLink(tag, payload)
	case TFSYNC:
		return s.handleFsync(tag, payload)
	case TLOCK:
		return s.handleLock(tag, payload)
	case TGETLOCK:
		return s.handleGetlock(tag, payload)
	case TSTATFS:
		return s.handleStatfs(tag, payload)
	case TXATTRWALK:
		return s.handleXattrwalk(tag, payload)
	case TXATTRCREATE:
		return s.handleXattrcreate(tag, payload)
	case TFLUSH:
		return s.handleFlush(tag, payload)
	case TAUTH:
		return nil, errUnsupported
	default:
		return nil, errUnsupported
	}
}

// ReleaseAll releases all fids. Call on server shutdown.
func (s *Server) ReleaseAll() {
	s.fids.ReleaseAll()
}
