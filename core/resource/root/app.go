package resource_root

import (
	"context"
	"errors"
	"strings"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ulid"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/db/world"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
)

// AppStorage is a validated storage capability for a nested installation.
// EphemeralID distinguishes privately owned Worlds; supplied engines remain caller-owned.
type AppStorage struct {
	StorageID   string
	Engine      world.Engine
	Prefix      string
	EphemeralID string
}

// AppRuntime exposes the Resource service and HTTP route of a live installation.
type AppRuntime interface {
	srpc.Invoker
	GetHTTPPathPrefix() string
}

// MountAppFunc attaches to the ordinary runtime composed by the root controller.
type MountAppFunc func(context.Context, AppStorage) (AppRuntime, func(), error)

// SetMountAppFunc installs the app runtime composition before serving requests.
func (s *CoreRootServer) SetMountAppFunc(fn MountAppFunc) { s.mountApp = fn }

// MountApp mounts an isolated installation using exactly one supplied storage source.
func (s *CoreRootServer) MountApp(ctx context.Context, req *s4wave_root.MountAppRequest) (*s4wave_root.MountAppResponse, error) {
	// Require the nested app runtime before resolving its storage.
	if s.mountApp == nil {
		return nil, errors.New("nested app runtime is unavailable")
	}

	// Resolve the supplied storage source for the nested installation.
	var sources int
	binding := AppStorage{StorageID: req.GetStorageId(), Prefix: strings.TrimRight(req.GetObjectPrefix(), "/")}
	if binding.StorageID != "" {
		sources++
	}
	if req.GetEphemeral() {
		sources++
		binding.EphemeralID = ulid.NewULID()
	}
	if req.GetWorldResourceId() != 0 {
		sources++
		client, err := resource_server.MustGetResourceClientContext(ctx)
		if err != nil {
			return nil, err
		}
		value, err := client.GetResourceValue(req.GetWorldResourceId())
		if err != nil {
			return nil, err
		}
		engineResource, ok := value.(*resource_world.EngineResource)
		if !ok || binding.Prefix == "" {
			return nil, errors.New("world storage requires an Engine Resource and object prefix")
		}
		binding.Engine = engineResource.GetEngine()
	}
	if sources != 1 {
		return nil, errors.New("select exactly one app storage source")
	}

	// Attach the nested runtime as a child resource of the caller.
	var httpPathPrefix string
	_, id, err := resource_server.ConstructChildResource(ctx, func(_ context.Context) (srpc.Invoker, struct{}, func(), error) {
		// Startup follows the request's cancellation. The runtime registry retains
		// shared installations independently once the caller owns its attachment.
		mux, release, err := s.mountApp(ctx, binding)
		if err == nil {
			httpPathPrefix = mux.GetHTTPPathPrefix()
		}
		return mux, struct{}{}, release, err
	})
	if err != nil {
		return nil, err
	}
	return &s4wave_root.MountAppResponse{ResourceId: id, HttpPathPrefix: httpPathPrefix}, nil
}
