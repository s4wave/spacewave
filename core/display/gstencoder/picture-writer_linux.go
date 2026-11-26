package gstencoder

import (
	"encoding/binary"
	"errors"
	"io"
	"slices"

	"github.com/aperturerobotics/protobuf-go-lite/encoding/protowire"
	"github.com/s4wave/spacewave/core/frametrack"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	"golang.org/x/sys/unix"
)

// PictureWriter writes the helper's FramePackets from one goroutine.
type PictureWriter struct {
	// fd is the output pipe owned by the helper.
	fd int
	// prefix holds the length and protobuf metadata and is reused across pictures.
	prefix []byte
}

// NewPictureWriter binds the writer to the helper's output descriptor.
func NewPictureWriter(fd int) *PictureWriter {
	return &PictureWriter{fd: fd}
}

// WriteFrame writes one FramePacket without copying frame.Data into the prefix.
func (w *PictureWriter) WriteFrame(header *frametrack.GroupHeader, frame *frametrack.Frame) error {
	// Compute the encoded length without inspecting the coded bytes.
	metadata := *frame
	metadata.Data = nil
	metadataSize := metadata.SizeVT()
	frameSize := metadataSize
	if len(frame.Data) != 0 {
		frameSize += 1 + protowire.SizeBytes(len(frame.Data))
	}

	// Bound the complete packet before growing the metadata buffer.
	payloadSize := 1 + protowire.SizeBytes(frameSize)
	var headerSize int
	if header != nil {
		headerSize = header.SizeVT()
		payloadSize += 1 + protowire.SizeBytes(headerSize)
	}
	if payloadSize > MaxPacketSize {
		return stream_packet.ErrMessageTooLarge
	}

	// Encode the small prefix and leave the access unit in GStreamer memory.
	w.prefix = append(w.prefix[:0], 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(w.prefix, uint32(payloadSize)) //nolint:gosec // MaxPacketSize bounds the length.
	if header != nil {
		w.prefix = appendSizedField(w.prefix, groupField, headerSize)
		start := len(w.prefix)
		w.prefix = slices.Grow(w.prefix, headerSize)[:start+headerSize]
		if _, err := header.MarshalToSizedBufferVT(w.prefix[start:]); err != nil {
			return err
		}
	}
	w.prefix = appendSizedField(w.prefix, frameField, frameSize)
	start := len(w.prefix)
	w.prefix = slices.Grow(w.prefix, metadataSize)[:start+metadataSize]
	if _, err := metadata.MarshalToSizedBufferVT(w.prefix[start:]); err != nil {
		return err
	}
	if len(frame.Data) != 0 {
		w.prefix = appendSizedField(w.prefix, dataField, len(frame.Data))
	}

	// Write the prefix and access unit together and finish any short write.
	return writePictureParts(w.fd, w.prefix, frame.Data)
}

// appendSizedField appends the lead bytes of one length-delimited protobuf field.
func appendSizedField(dst []byte, field protowire.Number, n int) []byte {
	return protowire.AppendVarint(protowire.AppendTag(dst, field, protowire.BytesType), uint64(n)) //nolint:gosec // n is a protobuf size or slice length.
}

// writePictureParts keeps both slices alive through writes of their remaining bytes.
func writePictureParts(fd int, prefix, data []byte) error {
	// Write both parts together on the normal path, preserving progress on EINTR.
	parts := [][]byte{prefix, data}
	for len(parts) > 0 && len(parts[0]) > 0 {
		n, err := unix.Writev(fd, parts)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}

		// A partial write may consume the prefix and only part of the picture.
		for len(parts) > 0 && n >= len(parts[0]) {
			n -= len(parts[0])
			parts = parts[1:]
		}
		if len(parts) > 0 {
			parts[0] = parts[0][n:]
		}
	}
	return nil
}
