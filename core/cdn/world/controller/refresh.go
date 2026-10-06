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

// Refresh queues a fetch after initial readiness. The controller retains the
// current engine on fetch failure and retries within its own lifetime.
func (c *Controller) Refresh(ctx context.Context, req *RefreshRequest) (*RefreshResponse, error) {
	if req.GetSpaceId() != "" && req.GetSpaceId() != c.conf.GetSpaceId() {
		return &RefreshResponse{}, nil
	}
	if _, err := c.GetWorldEngine(ctx); err != nil {
		return nil, err
	}
	c.refresh.RestartRoutine()
	return &RefreshResponse{Accepted: true}, nil
}
