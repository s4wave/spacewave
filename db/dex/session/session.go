// Package dex_session provides shared block transfer session logic for DEX
// backends. It wraps a bifrost stream/packet.Session with chunked block
// transfer, size validation, and hash verification.
package dex_session

import (
	"bytes"
	"io"
	"strconv"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// defaultChunkSize is the default chunk size in bytes (1KB).
const defaultChunkSize = 1024

// defaultMaxBlockSize is the default maximum block size in bytes (10MB).
//
// See block.MaxBlockSize for the rationale: 10 MiB is a sanity / DoS cap on a
// single wire-serialized block. Real block types (blob.Blob, byteslice chunks,
// IAVL nodes) are bounded by blob.DefChunkingMaxSize (768 KiB) and produce
// values well under this ceiling.
const defaultMaxBlockSize = block.MaxBlockSize

// DexSession wraps a stream/packet.Session for chunked block transfer.
type DexSession struct {
	sess      *stream_packet.Session
	chunkSize int
	maxBlock  uint64
}

// NewDexSession constructs a new DexSession wrapping the given stream.
// If chunkSize <= 0, uses the default (1KB).
// If maxBlockSize <= 0, uses the default (10MB).
func NewDexSession(stream io.ReadWriteCloser, chunkSize int, maxBlockSize uint64) *DexSession {
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}
	if maxBlockSize <= 0 {
		maxBlockSize = defaultMaxBlockSize
	}
	// maxMessageSize must accommodate a single chunk plus proto framing overhead.
	// We always chunk, so the largest single message is chunkSize + BlockTransfer fields.
	maxMsg := uint32(chunkSize) + 1024 //nolint:gosec
	return &DexSession{
		sess:      stream_packet.NewSession(stream, maxMsg),
		chunkSize: chunkSize,
		maxBlock:  maxBlockSize,
	}
}

// SendInit sends an init message with the block reference and total size.
func (d *DexSession) SendInit(requestID uint64, ref *block.BlockRef, totalSize uint64) error {
	msg := &BlockTransfer{
		RequestId: requestID,
		Ref:       ref,
		TotalSize: totalSize,
	}
	return d.sess.SendMsg(msg)
}

// SendChunk sends a data chunk message.
func (d *DexSession) SendChunk(requestID uint64, data []byte, complete bool) error {
	msg := &BlockTransfer{
		RequestId: requestID,
		Data:      data,
		Complete:  complete,
	}
	return d.sess.SendMsg(msg)
}

// SendCancel sends a cancel message for the given request.
func (d *DexSession) SendCancel(requestID uint64) error {
	msg := &BlockTransfer{
		RequestId: requestID,
		Cancel:    true,
	}
	return d.sess.SendMsg(msg)
}

// SendError sends an error message for the given request.
func (d *DexSession) SendError(requestID uint64, errMsg string) error {
	msg := &BlockTransfer{
		RequestId: requestID,
		Error:     errMsg,
	}
	return d.sess.SendMsg(msg)
}

// ReadMessage reads the next BlockTransfer from the stream.
func (d *DexSession) ReadMessage() (*BlockTransfer, error) {
	msg := &BlockTransfer{}
	if err := d.sess.RecvMsg(msg); err != nil {
		return nil, err
	}
	return msg, nil
}

// SendBlock sends a complete block as Init + chunked data messages.
// The data is always chunked even if it fits in a single chunk.
func (d *DexSession) SendBlock(requestID uint64, ref *block.BlockRef, data []byte) error {
	if err := d.SendInit(requestID, ref, uint64(len(data))); err != nil {
		return errors.Wrap(err, "send init")
	}

	for i := 0; i < len(data); i += d.chunkSize {
		end := min(i+d.chunkSize, len(data))
		complete := end >= len(data)
		if err := d.SendChunk(requestID, data[i:end], complete); err != nil {
			return errors.Wrap(err, "send chunk")
		}
	}

	// Handle empty data: send a single empty chunk with complete=true.
	if len(data) == 0 {
		if err := d.SendChunk(requestID, nil, true); err != nil {
			return errors.Wrap(err, "send chunk")
		}
	}

	return nil
}

// ReceiveBlock reads a complete block from the stream (Init + chunks).
// If maxBlockSize is 0, uses the session default.
// Returns the request ID, block reference, assembled data, and any error.
func (d *DexSession) ReceiveBlock(maxBlockSize uint64) (uint64, *block.BlockRef, []byte, error) {
	if maxBlockSize == 0 {
		maxBlockSize = d.maxBlock
	}

	// Read the init message.
	init, err := d.ReadMessage()
	if err != nil {
		return 0, nil, nil, errors.Wrap(err, "read init")
	}
	if init.GetRef() == nil {
		return init.GetRequestId(), nil, nil, errors.New("init message missing block ref")
	}
	if init.GetCancel() {
		return init.GetRequestId(), init.GetRef(), nil, errors.New("received cancel instead of init")
	}
	if init.GetError() != "" {
		return init.GetRequestId(), init.GetRef(), nil, errors.New(init.GetError())
	}

	totalSize := init.GetTotalSize()
	if totalSize > maxBlockSize {
		return init.GetRequestId(), init.GetRef(), nil, errors.Errorf(
			"block size %s exceeds max %s",
			strconv.FormatUint(totalSize, 10),
			strconv.FormatUint(maxBlockSize, 10),
		)
	}

	requestID := init.GetRequestId()
	ref := init.GetRef()

	// Read chunks into a buffer.
	var buf bytes.Buffer
	buf.Grow(int(totalSize)) //nolint:gosec
	for {
		msg, rerr := d.ReadMessage()
		if rerr != nil {
			return requestID, ref, nil, errors.Wrap(rerr, "read chunk")
		}
		if msg.GetCancel() {
			return requestID, ref, nil, errors.New("transfer cancelled")
		}
		if msg.GetError() != "" {
			return requestID, ref, nil, errors.New(msg.GetError())
		}

		chunk := msg.GetData()
		if len(chunk) > 0 {
			buf.Write(chunk)
			if uint64(buf.Len()) > totalSize { //nolint:gosec
				return requestID, ref, nil, errors.Errorf(
					"accumulated data %s exceeds declared size %s",
					strconv.FormatUint(uint64(buf.Len()), 10), //nolint:gosec
					strconv.FormatUint(totalSize, 10),
				)
			}
		}

		if msg.GetComplete() {
			break
		}
	}

	data := buf.Bytes()

	// Verify hash.
	if err := ref.VerifyData(data, true); err != nil {
		return requestID, ref, nil, errors.Wrap(err, "block verification failed")
	}

	return requestID, ref, data, nil
}

// Close closes the underlying session stream.
func (d *DexSession) Close() error {
	return d.sess.Close()
}
