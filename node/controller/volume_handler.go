package node_controller

import (
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/volume"
)

// volumeRefHandler implements the reference handler for LookupVolume.
type volumeRefHandler struct {
	c *Controller
}

func newVolumeRefHandler(c *Controller) *volumeRefHandler {
	return &volumeRefHandler{c: c}
}

// HandleValueAdded is called when a value is added to the directive.
// Should not block.
func (r *volumeRefHandler) HandleValueAdded(
	i directive.Instance,
	av directive.AttachedValue,
) {
	v, ok := av.GetValue().(volume.Volume)
	if !ok {
		return
	}
	vID := v.GetID()
	r.c.mtx.Lock()
	if vb, ok := r.c.volumes[vID]; !ok || vb != v {
		r.c.le.WithField("volume-id", vID).Debug("volume acquired")
		r.c.volumes[vID] = v
		for _, b := range r.c.buckets {
			b.PushVolume(vID)
		}
	}
	r.c.mtx.Unlock()
}

// HandleValueRemoved is called when a value is removed from the directive.
// Should not block.
func (r *volumeRefHandler) HandleValueRemoved(
	i directive.Instance,
	av directive.AttachedValue,
) {
	v, ok := av.GetValue().(volume.Volume)
	if !ok {
		return
	}
	vID := v.GetID()
	r.c.mtx.Lock()
	if vb, ok := r.c.volumes[vID]; ok && vb == v {
		r.c.le.WithField("volume-id", vID).Debug("volume removed")
		delete(r.c.volumes, vID)
		for _, b := range r.c.buckets {
			b.ClearVolume(vID)
		}
	}
	r.c.mtx.Unlock()
}

// HandleInstanceDisposed is called when a directive instance is disposed.
// This will occur if Close() is called on the directive instance.
func (r *volumeRefHandler) HandleInstanceDisposed(i directive.Instance) {
	r.c.mtx.Lock()
	for k := range r.c.volumes {
		delete(r.c.volumes, k)
	}
	r.c.mtx.Unlock()
}

// _ is a type assertion
var _ directive.ReferenceHandler = ((*volumeRefHandler)(nil))
