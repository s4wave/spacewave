package process_binding

import "github.com/aperturerobotics/util/broadcast"

// BindingRegistry reports committed process binding changes to resources of
// one Resource root. Storage remains authoritative across daemon restarts.
type BindingRegistry struct {
	// bcast reports that a binding snapshot may have changed.
	bcast broadcast.Broadcast
}

// NewBindingRegistry constructs a process binding change registry.
func NewBindingRegistry() *BindingRegistry {
	return &BindingRegistry{}
}

// Changed returns a channel closed after the next committed binding change.
func (r *BindingRegistry) Changed() <-chan struct{} {
	var ch <-chan struct{}
	r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ch = getWaitCh()
	})
	return ch
}

// NotifyChanged wakes binding watches after a committed decision.
func (r *BindingRegistry) NotifyChanged() {
	r.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		broadcastFn()
	})
}
