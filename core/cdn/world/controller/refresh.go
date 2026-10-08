package cdn_world_controller

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
)

// WorldRefreshServiceID addresses the refresh owner for one mounted engine.
func WorldRefreshServiceID(engineID string) string {
	return engineID + "/" + SRPCWorldRefreshServiceID
}

// InvokeMethod serves refresh requests on this mount's RPC service.
func (c *Controller) InvokeMethod(serviceID, methodID string, stream srpc.Stream) (bool, error) {
	handler := NewSRPCWorldRefreshHandler(c, WorldRefreshServiceID(c.conf.GetEngineId()))
	return handler.InvokeMethod(serviceID, methodID, stream)
}

// Refresh queues a fetch after the mount is running, including while the
// mounted head is still missing. The controller retains the current engine on
// fetch failure and retries within its own lifetime.
func (c *Controller) Refresh(ctx context.Context, req *RefreshRequest) (*RefreshResponse, error) {
	// Ignore invalidations for another Space.
	if req.GetSpaceId() != "" && req.GetSpaceId() != c.conf.GetSpaceId() {
		return &RefreshResponse{}, nil
	}

	// Wait for the mount to serve a head or watch for its first publication.
	_, err := c.ctr.WaitValueWithValidator(ctx, func(state *mountState) (bool, error) {
		return state != nil && (state.engine != nil || isMissingPublishedHead(state.err)), nil
	}, nil)
	if err != nil {
		return nil, err
	}

	// Fetch the pointer through the mount's existing refresh routine.
	c.refresh.RestartRoutine()
	return &RefreshResponse{Accepted: true}, nil
}
