package resource_cdn

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/cdn"
	"github.com/sirupsen/logrus"
)

// ErrUnknownCdn is returned when Lookup is called with an unregistered
// cdn_id. Today only the default slot is populated.
var ErrUnknownCdn = errors.New("unknown cdn id")

// Registry owns the process-scoped map of CdnInstances keyed by cdn_id.
// The default instance (empty id) is lazily constructed on first lookup
// against the production or SPACEWAVE_CDN_SPACE_ID-overridden Space id.
// Future configs can register additional instances via a constructor
// extension once the first non-default CDN is introduced.
type Registry struct {
	le *logrus.Entry
	b  bus.Bus

	ctx       context.Context
	ctxCancel context.CancelFunc

	mtx       sync.Mutex
	instances map[string]*CdnInstance
}

// NewRegistry constructs a Registry. The returned registry owns a detached
// lifecycle context used to scope any instance-level background routines
// (e.g. refresh routines). Close cancels that context and tears down all
// registered instances. The CDN singleton is anonymous: no peer id is
// required because the CDN Space is not part of any session's provider
// account.
func NewRegistry(le *logrus.Entry, b bus.Bus) *Registry {
	ctx, cancel := context.WithCancel(context.Background())
	return &Registry{
		le:        le,
		b:         b,
		ctx:       ctx,
		ctxCancel: cancel,
		instances: make(map[string]*CdnInstance),
	}
}

// Lookup returns the CdnInstance registered under cdnID, constructing the
// default slot on first call. Unknown non-default ids return ErrUnknownCdn.
func (r *Registry) Lookup(cdnID string) (*CdnInstance, error) {
	if cdnID != "" && cdnID != cdn.SpaceID() {
		return nil, errors.Wrapf(ErrUnknownCdn, "cdn id %q", cdnID)
	}

	r.mtx.Lock()
	defer r.mtx.Unlock()
	if inst, ok := r.instances[cdnID]; ok {
		return inst, nil
	}

	inst, err := newCdnInstance(r.ctx, r.le, r.b, cdn.SpaceID())
	if err != nil {
		return nil, err
	}
	r.instances[cdnID] = inst
	return inst, nil
}

// NotifyRootChanged wakes up the CdnInstance whose SpaceID matches spaceID
// so the downstream block store cache is invalidated and the cached snapshot
// is refreshed. Called when an upstream cdn-root-changed signal arrives
// (e.g. the session-level WS frame). Unknown spaceIDs are silently ignored
// so future CDNs that this process has not registered yet do not produce
// spurious errors. Returns true when a matching instance was found.
func (r *Registry) NotifyRootChanged(spaceID string) bool {
	if spaceID == "" {
		return false
	}
	r.mtx.Lock()
	defer r.mtx.Unlock()
	for _, inst := range r.instances {
		if inst.GetSpaceID() == spaceID {
			inst.Refresh()
			return true
		}
	}
	return false
}

// Close tears down every registered instance and cancels the registry
// lifecycle context. Safe to call more than once.
func (r *Registry) Close() {
	r.mtx.Lock()
	instances := r.instances
	r.instances = nil
	r.mtx.Unlock()

	for _, inst := range instances {
		inst.Close()
	}
	r.ctxCancel()
}
