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
	datLen := msg.SizeVT()
	outBuf := make([]byte, 0, datLen+9)
	// Ignore gosec linter here: SizeVT will never exceed uint64 max.
	outBuf = protobuf_go_lite.AppendVarint(outBuf, uint64(datLen)) //nolint:gosec
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
	b := make([]byte, 4)
	var err error
	_, err = readAtLeast(r, 0, 4, b)
	if err != nil {
		return nil, err
	}

	// Read the header length varint
	headerLen, headerLenBytes := protobuf_go_lite.ConsumeVarint(b)
	if headerLenBytes <= 0 {
		return nil, errors.New("invalid stream establish varint prefix")
	}
	if headerLenBytes > len(b) { // this should not be possible
		headerLenBytes = len(b)
	}

	// use MaxInt32 for compat with 32-bit systems
	if headerLen > math.MaxInt32 {
		return nil, errors.New("header too large: exceeds maximum uint32 value")
	}

	// header len is at most 100,000 bytes
	if headerLen > streamEstablishMaxPacketSize || headerLen == 0 {
		return nil, errors.Errorf(
			"stream establish header length invalid: %d (expected <= %d)",
			headerLen,
			streamEstablishMaxPacketSize,
		)
	}

	headerBuf := make([]byte, int(headerLen))
	copy(headerBuf, b[headerLenBytes:])
	n := len(b) - headerLenBytes
	if _, err := readAtLeast(r, n, int(headerLen), headerBuf); err != nil {
		return nil, err
	}

	// decode stream establish header
	estHeader := &StreamEstablish{}
	if err := estHeader.UnmarshalVT(headerBuf); err != nil {
		return nil, err
	}

	return estHeader, nil
}
