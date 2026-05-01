package resource_cdn

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	cdn_copy "github.com/s4wave/spacewave/core/cdn"
	cdn_sharedobject "github.com/s4wave/spacewave/core/cdn/sharedobject"
	resource_space "github.com/s4wave/spacewave/core/resource/space"
	space_resolve "github.com/s4wave/spacewave/core/space/resolve"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/world"
	s4wave_cdn "github.com/s4wave/spacewave/sdk/cdn"
	"github.com/sirupsen/logrus"
)

// CdnResource is a per-mount handle implementing CdnResourceService for a
// single CdnInstance. The resource does not own the instance; it is a thin
// adapter so the root GetCdn RPC (added in a later iteration) can hand out
// a resource reference without exposing the Registry directly.
type CdnResource struct {
	le       *logrus.Entry
	b        bus.Bus
	mux      srpc.Invoker
	instance *CdnInstance
}

// NewCdnResource constructs a CdnResource bound to the supplied instance.
// The caller retains ownership of the instance; CdnResource does not tear
// it down on its own.
func NewCdnResource(le *logrus.Entry, b bus.Bus, instance *CdnInstance) *CdnResource {
	r := &CdnResource{le: le, b: b, instance: instance}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return s4wave_cdn.SRPCRegisterCdnResourceService(mux, r)
	})
	return r
}

// GetMux returns the rpc mux.
func (r *CdnResource) GetMux() srpc.Invoker {
	return r.mux
}

// GetCdnSpaceId returns the CDN Space ULID this resource is bound to.
func (r *CdnResource) GetCdnSpaceId(
	_ context.Context,
	_ *s4wave_cdn.GetCdnSpaceIdRequest,
) (*s4wave_cdn.GetCdnSpaceIdResponse, error) {
	return &s4wave_cdn.GetCdnSpaceIdResponse{
		SpaceId: r.instance.GetSpaceID(),
	}, nil
}

// MountCdnSpace mounts the CDN SharedObject as a read-only Space resource on
// the caller's client. Reuses the shared cdn_sharedobject.NewWorldEngine +
// NewCdnSpaceBody + resource_space.NewSpaceResource machinery that backs the
// CdnBodyType branch in resource_sobject.MountSharedObjectBody, so both
// mount paths produce structurally identical SpaceResources.
func (r *CdnResource) MountCdnSpace(
	ctx context.Context,
	_ *s4wave_cdn.MountCdnSpaceRequest,
) (*s4wave_cdn.MountCdnSpaceResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	cdnSO := r.instance.GetSharedObject()
	we, err := cdn_sharedobject.NewWorldEngine(ctx, r.le, r.b, cdnSO, space_world_optypes.LookupWorldOp)
	if err != nil {
		return nil, errors.Wrap(err, "build cdn world engine")
	}
	body := cdn_sharedobject.NewCdnSpaceBody(cdnSO, we)
	spaceResource := resource_space.NewSpaceResource(r.le, r.b, body)

	id, err := resourceCtx.AddResource(spaceResource.GetMux(), we.Release)
	if err != nil {
		we.Release()
		return nil, errors.Wrap(err, "add cdn space resource")
	}
	return &s4wave_cdn.MountCdnSpaceResponse{ResourceId: id}, nil
}

// CopyV86ImageToSpace copies a V86Image from this CDN Space into a user-owned
// destination Space identified by session index + destination space id.
// Source is a fresh read-only WorldEngine built from the bound CdnInstance;
// destination is resolved via space_resolve.ResolveSpace using the caller's
// session. Both engines are released before the RPC returns. Underlying
// copy semantics (metadata block + five asset edges, dedupe by target key)
// are handled by core/cdn.CopyV86ImageFromCdn.
func (r *CdnResource) CopyV86ImageToSpace(
	ctx context.Context,
	req *s4wave_cdn.CopyV86ImageToSpaceRequest,
) (*s4wave_cdn.CopyV86ImageToSpaceResponse, error) {
	if req.GetDstSpaceId() == "" {
		return nil, errors.New("dst_space_id is required")
	}
	if req.GetSrcObjectKey() == "" {
		return nil, errors.New("src_object_key is required")
	}
	if req.GetDstObjectKey() == "" {
		return nil, errors.New("dst_object_key is required")
	}

	cdnSO := r.instance.GetSharedObject()
	srcEngine, err := cdn_sharedobject.NewWorldEngine(ctx, r.le, r.b, cdnSO, space_world_optypes.LookupWorldOp)
	if err != nil {
		return nil, errors.Wrap(err, "build cdn source world engine")
	}
	defer srcEngine.Release()

	resolved, dstCleanup, err := space_resolve.ResolveSpace(ctx, r.b, req.GetSessionIdx(), req.GetDstSpaceId())
	if err != nil {
		return nil, errors.Wrap(err, "resolve destination space")
	}
	defer dstCleanup()

	src := world.NewEngineWorldState(srcEngine.Engine, false)
	dst := world.NewEngineWorldState(resolved.Engine, true)

	if err := cdn_copy.CopyV86ImageFromCdn(ctx, src, dst, req.GetSrcObjectKey(), req.GetDstObjectKey()); err != nil {
		return nil, err
	}
	return &s4wave_cdn.CopyV86ImageToSpaceResponse{}, nil
}

// _ is a type assertion.
var _ s4wave_cdn.SRPCCdnResourceServiceServer = (*CdnResource)(nil)
