package frametrack

import (
	"context"
	"io"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// GroupProtocolID identifies the server-initiated reliable group streams.
const GroupProtocolID = protocol.ID("alpha/frame-track/group/1")

// OpenGroupStream opens the next group's stream.
// Closing the stream must interrupt any pending write.
type OpenGroupStream func(ctx context.Context) (io.ReadWriteCloser, error)

// OpenMountedGroupStream returns an opener on the subscription's existing link.
// The caller keeps the subscription and its link alive while using the opener.
func OpenMountedGroupStream(control link.MountedStream) OpenGroupStream {
	mounted := control.GetLink()
	return func(ctx context.Context) (io.ReadWriteCloser, error) {
		group, err := mounted.OpenMountedStream(ctx, GroupProtocolID, stream.OpenOpts{})
		if err != nil {
			return nil, err
		}
		return group.GetStream(), nil
	}
}

// WriteStreamGroups carries each group on its own reliable packet stream.
func WriteStreamGroups(open OpenGroupStream) WriteGroup {
	return func(ctx context.Context, header *GroupHeader, drain func(func(*Frame) error) error) error {
		// Open the group and interrupt its I/O when the sender retires it.
		strm, err := open(ctx)
		if err != nil {
			return err
		}
		closeStream := func() {
			// Close interrupts a blocked write and ends the stream.
			_ = strm.Close()
		}
		stopClose := context.AfterFunc(ctx, closeStream)
		defer func() {
			if stopClose() {
				closeStream()
			}
		}()

		// Send the bounded header before sending any access units.
		session := stream_packet.NewSession(strm, maxGroupMessageSize)
		if header.SizeVT() > maxGroupMessageSize {
			return errors.New("frame group header is too large")
		}
		if err := session.SendMsg(header); err != nil {
			return err
		}

		// Finish each write before the sender releases that frame's slot.
		return drain(func(frame *Frame) error {
			if frame.SizeVT() > maxGroupMessageSize {
				return errors.New("frame is too large")
			}
			return session.SendMsg(frame)
		})
	}
}
