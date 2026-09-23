//go:build !js && !tinygo

package space_exec

import (
	"context"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/go-git/go-billy/v6/osfs"
	"github.com/pkg/errors"
	frontend "github.com/s4wave/spacewave/bldr/frontend"
	project_controller "github.com/s4wave/spacewave/bldr/project/controller"
	"github.com/s4wave/spacewave/core/transport"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_block_fs "github.com/s4wave/spacewave/db/unixfs/block/fs"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/protocol"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	stream_srpc_server "github.com/s4wave/spacewave/net/stream/srpc/server"
)

// PluginFrontend retains the compiler while its authoring Watch is open.
// Fetch and Send borrow that lifetime; ending Watch ends the execution grant.
type PluginFrontend struct {
	// service owns compiler state and the Vite transport.
	service frontend.SRPCFrontendServer
	// stop cancels the enclosing Forge handler after the author detaches.
	stop context.CancelFunc
	// bcast guards the single authoring Watch admission.
	bcast broadcast.Broadcast
	// attached prevents a second stream from taking over the same attachment.
	attached bool
}

// NewPluginFrontend binds one compiler to its authoring attachment lifetime.
func NewPluginFrontend(service frontend.SRPCFrontendServer, stop context.CancelFunc) *PluginFrontend {
	return &PluginFrontend{service: service, stop: stop}
}

// Watch owns the attachment until its stream ends, including startup failure.
func (f *PluginFrontend) Watch(request *frontend.WatchRequest, stream frontend.SRPCFrontend_WatchStream) error {
	var attached bool
	f.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		attached = f.attached
		f.attached = true
	})
	if attached {
		return errors.New("frontend authoring attachment is already in use")
	}
	defer f.stop()
	return f.service.Watch(request, stream)
}

// Send forwards a message to the retained compiler's current session.
func (f *PluginFrontend) Send(ctx context.Context, request *frontend.SendRequest) (*frontend.SendResponse, error) {
	return f.service.Send(ctx, request)
}

// Fetch forwards module requests through the existing compiler service.
func (f *PluginFrontend) Fetch(stream frontend.SRPCFrontend_FetchStream) error {
	return f.service.Fetch(stream)
}

// runFrontend serves the selected Session and follows accepted source roots.
// The Forge context and the authoring Watch both cancel and join the compiler.
func (h *buildPluginHandler) runFrontend(ctx context.Context, directory string, service *project_controller.FrontendService) error {
	if service == nil {
		return errors.New("project has no live frontend compiler")
	}
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	transportBus := h.bus
	if h.config.GetFrontendPeerId() != h.handle.GetPeerId().String() {
		var releaseTransport func()
		var err error
		transportBus, releaseTransport, err = transport.ResolveSessionBus(runCtx, h.bus, h.handle.GetPeerId(), stop)
		if err != nil {
			return err
		}
		defer releaseTransport()
	}
	attachment := NewPluginFrontend(service, stop)
	mux := srpc.NewMux()
	if err := frontend.SRPCRegisterFrontend(mux, attachment); err != nil {
		return err
	}

	// Ending the execution closes pending calls even when a caller stops reading.
	invoker := srpc.InvokerFunc(func(serviceID, methodID string, stream srpc.Stream) (bool, error) {
		callCtx, cancel := context.WithCancel(stream.Context())
		stopCancel := context.AfterFunc(runCtx, func() {
			cancel()
			// Context substitution does not interrupt the wrapped stream's reads.
			// Close also releases a Fetch still waiting for its first message.
			_ = stream.Close()
		})
		defer stopCancel()
		defer cancel()
		return mux.InvokeMethod(serviceID, methodID, srpc.NewStreamWithContext(stream, callCtx))
	})

	// The execution owns its compiler route until source watching ends.
	release, err := transportBus.AddController(runCtx, h.frontendServer(transportBus, invoker), nil)
	if err != nil {
		return err
	}
	defer release()

	// Source changes affect Vite's checkout, never the installed backend artifact.
	err = h.watchFrontendSource(runCtx, directory)
	if runCtx.Err() != nil && ctx.Err() == nil {
		return nil
	}
	return err
}

// frontendServer routes a local Session directly and authenticates remote callers.
func (h *buildPluginHandler) frontendServer(transportBus bus.Bus, invoker srpc.Invoker) controller.Controller {
	// Local callers already hold the mounted Session's Resource capability.
	info := controller.NewInfo("space/plugin-frontend", controller.MustParseVersion("0.0.1"), "Space plugin frontend attachment")
	protocolID := PluginFrontendProtocol(h.config.GetFrontendId())
	if h.config.GetFrontendPeerId() == h.handle.GetPeerId().String() {
		return bifrost_rpc.NewRpcServiceController(info, bifrost_rpc.NewRpcServiceBuilder(invoker),
			[]string{string(protocolID) + "/"}, true, nil, nil, nil)
	}

	// A remote stream must belong to the exact author named in the queued grant.
	authenticated := srpc.NewMux(srpc.InvokerFunc(func(serviceID, methodID string, stream srpc.Stream) (bool, error) {
		mounted, err := link.MustGetMountedStreamContext(stream.Context())
		if err != nil {
			return true, err
		}
		if mounted.GetPeerID().String() != h.config.GetFrontendPeerId() {
			return true, errors.New("frontend attachment belongs to another Session")
		}
		return invoker.InvokeMethod(serviceID, methodID, stream)
	}))
	return stream_srpc_server.NewServerWithMux(transportBus, h.le, info,
		authenticated, []protocol.ID{protocolID}, []string{h.handle.GetPeerId().String()}, false)
}

// watchFrontendSource copies complete accepted roots and retains compiler inputs.
func (h *buildPluginHandler) watchFrontendSource(ctx context.Context, directory string) error {
	ws := world.NewEngineWorldState(h.engine, false)
	object, err := world.MustGetObject(ctx, ws, h.source.GetKey())
	if err != nil {
		return err
	}
	defer world.ReleaseObjectState(object)
	previous := h.source.GetRootRef()
	for {
		root, revision, err := object.GetRootRef(ctx)
		if err != nil {
			return err
		}
		if !root.EqualVT(previous) {
			err = h.handle.AccessStorage(ctx, root, func(cursor *bucket_lookup.Cursor) error {
				filesystem := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, cursor.Clone(), nil)
				source, err := unixfs.NewFSHandle(filesystem)
				if err != nil {
					filesystem.Release()
					return err
				}
				defer source.Release()
				return unixfs_sync.SyncToBillyContents(ctx, osfs.New(directory), source, unixfs_sync.DeleteMode_DeleteMode_DURING,
					func(_ context.Context, name string, _ unixfs.FSCursorNodeType) (bool, error) {
						first, _, _ := strings.Cut(name, "/")
						return first != ".bldr" && first != "node_modules", nil
					})
			})
			if err != nil {
				return err
			}
			previous = root
		}
		if _, err := object.WaitRev(ctx, revision+1, false); err != nil {
			return err
		}
	}
}

var _ frontend.SRPCFrontendServer = (*PluginFrontend)(nil)
