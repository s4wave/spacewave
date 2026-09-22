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

// ErrRegistryClosed is returned when a lookup occurs after registry shutdown.
var ErrRegistryClosed = errors.New("cdn registry is closed")

// Registry owns one process-scoped instance per canonical CDN Space ID.
// The empty ID and configured Space ID resolve to the same lazy instance.
type Registry struct {
	// le records CDN failures.
	le *logrus.Entry
	// b resolves shared CDN resources.
	b bus.Bus
	// ctx bounds instance refresh routines until Close.
	ctx context.Context
	// ctxCancel ends the registry lifetime.
	ctxCancel context.CancelFunc
	// mtx guards instances and lazy construction.
	mtx sync.Mutex
	// instances holds mounted CDNs, or nil after Close.
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

// Lookup returns the shared instance for the empty alias or configured CDN ID.
// Unknown IDs return ErrUnknownCdn without initializing or fetching a CDN.
func (r *Registry) Lookup(cdnID string) (*CdnInstance, error) {
	// Validate before lazy construction and canonicalize both accepted names.
	spaceID := cdn.SpaceID()
	if cdnID != "" && cdnID != spaceID {
		return nil, errors.Wrapf(ErrUnknownCdn, "cdn id %q", cdnID)
	}

	// Serialize construction so all aliases share the same instance and cache.
	r.mtx.Lock()
	defer r.mtx.Unlock()
	if r.instances == nil {
		return nil, ErrRegistryClosed
	}
	if inst, ok := r.instances[spaceID]; ok {
		return inst, nil
	}

	inst, err := newCdnInstance(r.ctx, r.le, r.b, spaceID)
	if err != nil {
		return nil, err
	}
	r.instances[spaceID] = inst
	return inst, nil
}

// NotifyRootChanged wakes up the CdnInstance whose SpaceID matches spaceID
// so the downstream block store cache is invalidated and the cached snapshot
// is refreshed. Called when an upstream cdn-root-changed signal arrives
// (e.g. the session-level WS frame). Unknown spaceIDs are silently ignored
// so future CDNs that this process has not registered yet do not produce
// spurious errors. Returns true when a matching instance was found.
func (r *Registry) NotifyRootChanged(spaceID string) bool {
	// Ignore notifications without a Space identity.
	if spaceID == "" {
		return false
	}

	// Refresh only an instance already mounted by a consumer.
	r.mtx.Lock()
	defer r.mtx.Unlock()
	if inst := r.instances[spaceID]; inst != nil {
		inst.Refresh()
		return true
	}
	return false
}

// Close tears down every registered instance and cancels the registry
// lifecycle context. Safe to call more than once.
func (r *Registry) Close() {
	// Withdraw the registry before releasing any mounted instance.
	r.mtx.Lock()
	instances := r.instances
	r.instances = nil
	r.mtx.Unlock()

	// End refresh routines and their shared block-store caches.
	for _, inst := range instances {
		inst.Close()
	}
	r.ctxCancel()
}
