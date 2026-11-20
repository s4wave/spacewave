package unixfs_rpc_server

import (
	"io"

	"github.com/pkg/errors"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
)

// fileUploadReader delivers one file from a client stream, retaining at most one frame.
type fileUploadReader struct {
	// stream supplies the data messages and signals when the client is done.
	stream unixfs_rpc.SRPCFSCursorService_OpsMknodWithContentStream
	// remaining counts bytes expected from future frames.
	remaining int64
	// pending holds the unread part of the current frame.
	pending []byte
}

// Read returns file bytes and verifies the declared size against the stream end.
func (r *fileUploadReader) Read(p []byte) (int, error) {
	// Fetch the next frame only after consuming the current one.
	if len(r.pending) == 0 {
		msg, err := r.stream.Recv()
		if err == io.EOF && r.remaining != 0 {
			return 0, io.ErrUnexpectedEOF
		}
		if err != nil {
			return 0, err
		}
		r.pending = msg.GetData()
		if len(r.pending) == 0 || len(r.pending) > unixfs_rpc.FileUploadChunkSize || int64(len(r.pending)) > r.remaining {
			return 0, errors.New("invalid file upload data frame")
		}
		r.remaining -= int64(len(r.pending))
	}

	// Deliver as much of the retained frame as the caller can take.
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

// _ is a type assertion.
var _ io.Reader = (*fileUploadReader)(nil)
