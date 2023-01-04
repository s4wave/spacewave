package volume_rpc_client

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/directive"
)

// LoadProxyVolumeResolver loads a proxy volume for a directive.
// Exits once the proxy volume controller is fully running.
// Does not directly resolve the directive.
type LoadProxyVolumeResolver struct {
	// c is the controller
	c *Controller
	// di is the directive instance
	di directive.Instance
	// volumeID is the volume identifier
	volumeID string
	// refAdded indicates the reference was already added.
	refAdded atomic.Bool
}

// NewLoadProxyVolumeResolver constructs a new LoadProxyVolumeResolver.
func NewLoadProxyVolumeResolver(c *Controller, di directive.Instance, volumeID string) *LoadProxyVolumeResolver {
	if volumeID == "" {
		return nil
	}
	return &LoadProxyVolumeResolver{c: c, di: di, volumeID: volumeID}
}

// Resolve resolves the values, emitting them to the handler.
func (r *LoadProxyVolumeResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	if r.refAdded.Swap(true) {
		return nil
	}

	volumeID := r.volumeID
	le := r.c.le.WithField("volume-id", r.volumeID)

	le.Debug("adding proxy volume reference")
	ref, tracker, _ := r.c.proxyVolumes.AddKeyRef(volumeID)
	r.di.AddDisposeCallback(func() {
		le.Debug("removed proxy volume reference")
		r.refAdded.Store(false)
		ref.Release()
	})

	// wait for the volume to be ready
	// (note: only error it can return is context.Canceled)
	_, err := tracker.proxyVolCtr.WaitValue(ctx, nil)
	return err
}

// _ is a type assertion
var _ directive.Resolver = ((*LoadProxyVolumeResolver)(nil))
