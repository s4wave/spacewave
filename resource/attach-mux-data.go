package resource

import (
	"bytes"
	"io"
	"slices"
)

// AttachMuxDataRwc bridges ResourceAttach mux_data packets to io.ReadWriteCloser.
// Used by both server and client sides of ResourceAttach to provide a
// ReadWriteCloser for yamux over the bidi stream.
type AttachMuxDataRwc struct {
	// sendMuxData sends a mux_data packet.
	sendMuxData func(data []byte) error
	// recvMuxData receives a mux_data packet (blocks until data arrives).
	recvMuxData func() ([]byte, error)
	// buf holds partial mux_data from a previous recv.
	buf bytes.Buffer
}

// NewAttachMuxDataRwc constructs a new AttachMuxDataRwc.
// sendMuxData sends raw bytes as a mux_data packet.
// recvMuxData blocks until the next mux_data packet arrives and returns its bytes.
func NewAttachMuxDataRwc(
	sendMuxData func(data []byte) error,
	recvMuxData func() ([]byte, error),
) *AttachMuxDataRwc {
	return &AttachMuxDataRwc{
		sendMuxData: sendMuxData,
		recvMuxData: recvMuxData,
	}
}

// Read reads from buffered mux_data, fetching more from recvMuxData as needed.
func (a *AttachMuxDataRwc) Read(p []byte) (int, error) {
	for a.buf.Len() == 0 {
		data, err := a.recvMuxData()
		if err != nil {
			return 0, err
		}
		if len(data) == 0 {
			continue
		}
		a.buf.Write(data)
	}
	return a.buf.Read(p)
}

// Write sends p as a mux_data packet. Clones the data since yamux may reuse buffers.
func (a *AttachMuxDataRwc) Write(p []byte) (int, error) {
	if err := a.sendMuxData(slices.Clone(p)); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Close is a no-op; the stream lifecycle is managed by the caller.
func (a *AttachMuxDataRwc) Close() error {
	return nil
}

// _ is a type assertion
var _ io.ReadWriteCloser = (*AttachMuxDataRwc)(nil)
