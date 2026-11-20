package s4wave_device

import (
	"context"
	"slices"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_unixfs "github.com/s4wave/spacewave/core/resource/unixfs"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
	unixfs_rpc_client "github.com/s4wave/spacewave/db/unixfs/rpc/client"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
)

// openLiveCheckoutRoot connects the selected Device protocol to an FSHandle resource.
func (r *DeviceResource) openLiveCheckoutRoot(ctx context.Context, owner resource_server.ResourceClientContext, device *Device, id protocol.ID, write bool) (_ uint32, rerr error) {
	// Resolve the Device from its current peer and prepare the handle lifetime.
	remote, err := peer.IDB58Decode(device.GetPeerId())
	if err != nil {
		return 0, errors.Wrap(err, "checkout-root Device peer ID")
	}
	liveCtx, cancel := context.WithCancel(owner.Context())
	stopOpen := context.AfterFunc(ctx, cancel)
	defer stopOpen()
	cleanup := []func(){cancel}
	release := func() {
		for _, fn := range slices.Backward(cleanup) {
			fn()
		}
	}
	defer func() {
		if rerr != nil {
			release()
		}
	}()

	// Borrow the selected Session until the filesystem handle is released.
	st, releaseSession, err := transport.ResolveSessionTransport(liveCtx, r.bus, r.sessionPeerID, cancel)
	if err != nil {
		return 0, err
	}
	cleanup = append(cleanup, releaseSession)
	if st == nil {
		return 0, errors.New("the viewer's Session transport stopped")
	}
	sessionBus := st.GetChildBus()
	if sessionBus == nil {
		return 0, errors.New("the viewer's Session transport stopped")
	}

	// Hold a single Bifrost stream for this handle's cursor event and operation RPCs.
	ms, releaseLink, err := link.OpenStreamWithPeerEx(liveCtx, sessionBus, id, r.sessionPeerID, remote, 0, stream.OpenOpts{})
	if err != nil {
		return 0, err
	}
	cleanup = append(cleanup, releaseLink)
	strm := ms.GetStream()
	stopClose := context.AfterFunc(liveCtx, func() { _ = strm.Close() })
	mc, err := srpc.NewMuxedConnWithRwc(liveCtx, strm, true, nil)
	if err != nil {
		return 0, err
	}
	cleanup = append(cleanup, func() {
		stopClose()
		_ = mc.Close()
	})

	// Wait for the admitted cursor before returning a handle to the caller.
	client := unixfs_rpc.NewSRPCFSCursorServiceClient(srpc.NewClientWithMuxedConn(mc))
	cursor, err := unixfs_rpc_client.BuildFSCursorClient(liveCtx, client)
	if err != nil {
		return 0, errors.Wrap(err, "the Device did not open the checkout root")
	}
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		cursor.Release()
		return 0, err
	}
	cleanup = append(cleanup, handle.Release)
	if _, err := handle.GetNodeType(liveCtx); err != nil {
		return 0, err
	}
	if !stopOpen() {
		return 0, context.Canceled
	}

	// Transfer the handle, transport and cursor connection to the Resource client.
	newResource := resource_unixfs.NewReadOnlyFSHandleResource
	if write {
		newResource = resource_unixfs.NewFSHandleResource
	}
	res := newResource(handle)
	resourceID, err := owner.AddResourceValue(res.GetMux(), res, release)
	if err != nil {
		return 0, err
	}

	// Ending the cursor event stream invalidates the resource and its descendants.
	context.AfterFunc(cursor.Context(), func() { owner.ReleaseResource(resourceID) })
	return resourceID, nil
}
