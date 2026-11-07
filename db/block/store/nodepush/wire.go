package block_store_nodepush

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"io"

	"github.com/pkg/errors"
)

// maxMessageBytes bounds one frame. A publication's blocks fit well inside it;
// a larger one is left to the local upload.
const maxMessageBytes = 16 << 20

// encodeMessage returns msg as one frame.
func encodeMessage(msg *Message) ([]byte, error) {
	// Encode the body behind its length.
	body, err := msg.MarshalVT()
	if err != nil {
		return nil, err
	}
	if len(body) > maxMessageBytes {
		return nil, errors.Errorf("frame of %d bytes exceeds the %d byte limit", len(body), maxMessageBytes)
	}
	return append(binary.AppendUvarint(nil, uint64(len(body))), body...), nil
}

// readMessage reads one frame.
func readMessage(r *bufio.Reader) (*Message, error) {
	// Read the length, then the body it names.
	size, err := binary.ReadUvarint(r)
	if err != nil {
		return nil, err
	}
	if size > maxMessageBytes {
		return nil, errors.Errorf("frame of %d bytes exceeds the %d byte limit", size, maxMessageBytes)
	}
	body := make([]byte, size)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	msg := &Message{}
	if err := msg.UnmarshalVT(body); err != nil {
		return nil, err
	}
	return msg, nil
}

// blocksSum returns the sum a Push carries of blocks.
func blocksSum(blocks []*Block) []byte {
	// Length-prefix each part, so moving a byte across a boundary changes it.
	h := sha256.New()
	var size [binary.MaxVarintLen64]byte
	for _, b := range blocks {
		for _, part := range [][]byte{b.GetKey(), b.GetData()} {
			h.Write(size[:binary.PutUvarint(size[:], uint64(len(part)))])
			h.Write(part)
		}
	}
	return h.Sum(nil)
}
