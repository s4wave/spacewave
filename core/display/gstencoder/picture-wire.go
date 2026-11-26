package gstencoder

import (
	"encoding/binary"
	"errors"
	"io"

	"github.com/aperturerobotics/protobuf-go-lite/encoding/protowire"
	"github.com/s4wave/spacewave/core/frametrack"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

const (
	// groupField identifies FramePacket.header in the helper's protobuf.
	groupField protowire.Number = 1
	// frameField identifies FramePacket.frame in the helper's protobuf.
	frameField protowire.Number = 2
	// dataField identifies Frame.data, which the helper writes last.
	dataField protowire.Number = 3
)

// readPicture reads one packet into memory that stays owned by its frame.
func readPicture(r io.Reader, prefix *[4]byte) (*frametrack.GroupHeader, *frametrack.Frame, error) {
	// Bound the immutable packet before allocating its payload.
	if _, err := io.ReadFull(r, prefix[:]); err != nil {
		return nil, nil, err
	}
	size := binary.LittleEndian.Uint32(prefix[:])
	if size > MaxPacketSize {
		return nil, nil, stream_packet.ErrMessageTooLarge
	}

	// A downstream viewer can retain Data while the next packet is read.
	body := make([]byte, size)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, nil, err
	}
	return decodePicture(body)
}

// decodePicture reads the optional group header followed by the frame.
func decodePicture(body []byte) (*frametrack.GroupHeader, *frametrack.Frame, error) {
	// The helper emits the opening group's metadata before the frame.
	var header *frametrack.GroupHeader
	if len(body) > 0 && uint64(body[0]) == protowire.EncodeTag(groupField, protowire.BytesType) {
		value, n := protowire.ConsumeBytes(body[1:])
		if n < 0 {
			return nil, nil, protowire.ParseError(n)
		}
		header = new(frametrack.GroupHeader)
		if err := header.UnmarshalVT(value); err != nil {
			return nil, nil, err
		}
		body = body[1+n:]
	}

	// The frame is the final field of the helper's FramePacket.
	if len(body) == 0 || uint64(body[0]) != protowire.EncodeTag(frameField, protowire.BytesType) {
		return nil, nil, errors.New("encoder packet has no frame")
	}
	value, n := protowire.ConsumeBytes(body[1:])
	if n < 0 {
		return nil, nil, protowire.ParseError(n)
	}
	if n != len(body)-1 {
		return nil, nil, errors.New("encoder packet has bytes after the frame")
	}
	frame, err := decodeFrame(value)
	return header, frame, err
}

// decodeFrame uses the generated decoder for metadata and borrows the coded bytes.
func decodeFrame(body []byte) (*frametrack.Frame, error) {
	// Only the final Data field uses the owned packet instead of a copy.
	frame := new(frametrack.Frame)
	for tail := body; len(tail) > 0; {
		number, typ, tagSize := protowire.ConsumeTag(tail)
		if tagSize < 0 {
			return nil, protowire.ParseError(tagSize)
		}
		if number == dataField && typ == protowire.BytesType {
			data, n := protowire.ConsumeBytes(tail[tagSize:])
			if n < 0 {
				return nil, protowire.ParseError(n)
			}
			if tagSize+n != len(tail) {
				return nil, errors.New("encoder frame has bytes after the picture")
			}
			frame.Data = data[:len(data):len(data)]
			body = body[:len(body)-len(tail)]
			break
		}
		n := protowire.ConsumeFieldValue(number, typ, tail[tagSize:])
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		tail = tail[tagSize+n:]
	}
	return frame, frame.UnmarshalVT(body)
}
