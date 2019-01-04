package node_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
)

type applyBucketConfHandler struct {
	c   *Controller
	ref directive.Reference
}

// handleApplyBucketConfig observes the apply directive and purges/pushes any
// relevant volume handles if necessary.
func (c *Controller) handleApplyBucketConfig(
	ctx context.Context,
	di directive.Instance,
	d bucket.ApplyBucketConfig,
) {
	h := &applyBucketConfHandler{c: c}
	h.ref = di.AddReference(h, true)
}

// HandleValueAdded is called when a value is added to the directive.
// Should not block.
func (h *applyBucketConfHandler) HandleValueAdded(
	di directive.Instance, av directive.AttachedValue,
) {
	val, ok := av.GetValue().(bucket.ApplyBucketConfigValue)
	if !ok {
		return
	}
	if val.GetUpdated() {
		h.c.flushBucketVolume(val.GetBucketId(), val.GetVolumeId())
	}
}

// HandleValueRemoved is called when a value is removed from the directive.
// Should not block.
func (h *applyBucketConfHandler) HandleValueRemoved(
	di directive.Instance, av directive.AttachedValue,
) {
	// noop
}

// HandleInstanceDisposed is called when a directive instance is disposed.
// This will occur if Close() is called on the directive instance.
func (h *applyBucketConfHandler) HandleInstanceDisposed(di directive.Instance) {
	// noop
}

// _ is a type assertion
var _ directive.ReferenceHandler = ((*applyBucketConfHandler)(nil))
