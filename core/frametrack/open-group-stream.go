package frametrack

import (
	"context"
	"io"

	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
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
