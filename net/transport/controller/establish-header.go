package transport_controller

import (
	"io"
	"math"

	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/protocol"
)

// NewStreamEstablish constructs a new StreamEstablish message.
func NewStreamEstablish(protocolID protocol.ID) *StreamEstablish {
	return &StreamEstablish{ProtocolId: string(protocolID)}
}

func marshalStreamEstablishHeader(msg *StreamEstablish) []byte {
	// Compute the payload size and allocate space for its length prefix.
	datLen := msg.SizeVT()
	outBuf := make([]byte, 0, datLen+9)

	// Ignore gosec linter here: SizeVT will never exceed uint64 max.
	// Encode the payload length as a varint prefix.
	outBuf = protobuf_go_lite.AppendVarint(outBuf, uint64(datLen)) //nolint:gosec

	// Reserve the payload area and marshal the message into it.
	prefixLen := len(outBuf)
	outBuf = outBuf[:len(outBuf)+datLen]
	msgFinalLen, _ := msg.MarshalToVT(outBuf[prefixLen:])
	return outBuf[:prefixLen+msgFinalLen]
}

func writeStreamEstablishHeader(w io.Writer, msg *StreamEstablish) (int, error) {
	return w.Write(marshalStreamEstablishHeader(msg))
}

func readAtLeast(r io.Reader, n, min int, buf []byte) (int, error) {
	for n < min {
		nr, err := r.Read(buf[n:])
		if err != nil {
			return n, err
		}
		n += nr
	}
	return n, nil
}

func readStreamEstablishHeader(r io.Reader) (*StreamEstablish, error) {
	// Read the fixed prefix bytes needed to decode the header length.
	b := make([]byte, 4)
	var err error
	_, err = readAtLeast(r, 0, 4, b)
	if err != nil {
		return nil, err
	}

	// Decode and validate the header length varint.
	headerLen, headerLenBytes := protobuf_go_lite.ConsumeVarint(b)
	if headerLenBytes <= 0 {
		return nil, errors.New("invalid stream establish varint prefix")
	}

	if headerLenBytes > len(b) { // this should not be possible
		headerLenBytes = len(b)
	}

	// Enforce the platform-safe maximum header size.
	if headerLen > math.MaxInt32 {
		return nil, errors.New("header too large: exceeds maximum uint32 value")
	}

	// Enforce the configured header size bounds.
	if headerLen > streamEstablishMaxPacketSize || headerLen == 0 {
		return nil, errors.Errorf(
			"stream establish header length invalid: %d (expected <= %d)",
			headerLen,
			streamEstablishMaxPacketSize,
		)
	}

	// Fill the header buffer from the prefix and remaining reader bytes.
	headerBuf := make([]byte, int(headerLen))
	copy(headerBuf, b[headerLenBytes:])
	n := len(b) - headerLenBytes
	if _, err := readAtLeast(r, n, int(headerLen), headerBuf); err != nil {
		return nil, err
	}

	// Decode the establishment message from the complete header.
	estHeader := &StreamEstablish{}
	if err := estHeader.UnmarshalVT(headerBuf); err != nil {
		return nil, err
	}

	return estHeader, nil
}
