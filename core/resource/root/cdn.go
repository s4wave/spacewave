package resource_root

import (
	"context"

	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_cdn "github.com/s4wave/spacewave/core/resource/cdn"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
)

// GetCdn mounts a CdnResource for the selected CDN instance and returns both
// the client resource id and the CDN Space ULID in one round-trip. Empty
// cdn_id selects the default CDN. Unknown ids return the wrapped
// ErrUnknownCdn from the registry so callers can distinguish misconfigured
// ids from transport errors.
func (s *CoreRootServer) GetCdn(
	ctx context.Context,
	req *s4wave_root.GetCdnRequest,
) (*s4wave_root.GetCdnResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	instance, err := s.cdnRegistry.Lookup(req.GetCdnId())
	if err != nil {
		return nil, err
	}

	cdnResource := resource_cdn.NewCdnResource(s.le, s.b, instance)
	id, err := resourceCtx.AddResource(cdnResource.GetMux(), func() {})
	if err != nil {
		return nil, err
	}

	return &s4wave_root.GetCdnResponse{
		ResourceId: id,
		CdnSpaceId: instance.GetSpaceID(),
	}, nil
}

// lookupCdnSharedObject returns the CDN SharedObject and its synthesized
// metadata when sharedObjectID matches the default CDN Space, otherwise
// returns (nil, nil). Wired into each mounted SessionResource via
// SetCdnLookup so MountSharedObject can return the anonymous singleton
// instead of surfacing not-found from the per-session shared object list
// (which filters CDN Spaces out).
func (s *CoreRootServer) lookupCdnSharedObject(
	sharedObjectID string,
) (sobject.SharedObject, *sobject.SharedObjectMeta) {
	if sharedObjectID == "" {
		return nil, nil
	}
	inst, err := s.cdnRegistry.Lookup("") // empty = default ID
	if err != nil || inst == nil {
		return nil, nil
	}
	if inst.GetSpaceID() != sharedObjectID {
		return nil, nil
	}
	cdnSO := inst.GetSharedObject()
	return cdnSO, cdnSO.GetMeta()
}
