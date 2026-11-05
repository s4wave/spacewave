package resource_space

import (
	"context"
	"slices"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/broadcast"
	space_world "github.com/s4wave/spacewave/core/space/world"
)

// opPluginLoaderControllerID identifies the operation plugin loader.
const opPluginLoaderControllerID = "spacewave/space/op-plugin-loader"

// opPluginLoader runs the plugins that replay of the Space's World asks for
// with LoadOpPlugins, from the first request until the Space resource is
// released. A plugin keeps running after its handler resolves, so later
// operations of its types do not each pause replay and restart it. Only the
// Space's listed plugins whose manifests declare an operation type the World
// replays are ever requested.
type opPluginLoader struct {
	// r is the Space resource whose plugin runtime runs the plugins.
	r *SpaceResource
	// bcast guards pluginIDs.
	bcast broadcast.Broadcast
	// pluginIDs is the sorted set of requested plugins.
	pluginIDs []string
}

// StartOpPluginLoader runs the plugins that replay of the Space's World
// waits for, until the returned release is called. ctx is the mount's
// context: the loader keeps its values, which select the same plugin runtime
// an application of this resource acquires, but not its cancellation.
func (r *SpaceResource) StartOpPluginLoader(ctx context.Context) func() {
	// Watch for requests from the Space's World engine.
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	l := &opPluginLoader{r: r}
	releaseCtrl, err := r.b.AddController(ctx, l, nil)
	if err != nil {
		cancel()
		r.le.WithError(err).Warn("cannot start the operation plugin loader")
		return func() {}
	}

	// Run the requested plugins until release.
	go l.run(ctx)
	return func() {
		releaseCtrl()
		cancel()
	}
}

// GetControllerInfo returns information about the controller.
func (l *opPluginLoader) GetControllerInfo() *controller.Info {
	return controller.NewInfo(opPluginLoaderControllerID, controller.MustParseVersion("0.0.1"), "Space operation plugin loader")
}

// Execute returns immediately, since the loader only handles directives.
func (l *opPluginLoader) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective adds the plugins a LoadOpPlugins for the Space's World
// engine requests. It resolves no value.
func (l *opPluginLoader) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	// Accept only requests from this Space's World engine.
	dir, ok := di.GetDirective().(space_world.LoadOpPlugins)
	if !ok || dir.LoadOpPluginsEngineID() != l.r.GetWorldEngineID() {
		return nil, nil
	}

	// Add the requested plugins and wake the runner.
	l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		ids := slices.Concat(l.pluginIDs, dir.LoadOpPluginsPluginIDs())
		slices.Sort(ids)
		ids = slices.Compact(ids)
		if len(ids) != len(l.pluginIDs) {
			l.pluginIDs = ids
			broadcast()
		}
	})
	return nil, nil
}

// Close releases nothing.
func (l *opPluginLoader) Close() error {
	return nil
}

// getPluginIDs returns the requested plugins and a channel closed when they
// change.
func (l *opPluginLoader) getPluginIDs() ([]string, <-chan struct{}) {
	var ids []string
	var ch <-chan struct{}
	l.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ids, ch = l.pluginIDs, getWaitCh()
	})
	return ids, ch
}

// run waits for the first request, then demands the requested plugins from
// the Space plugin runtime until ctx ends, retrying a failed acquire.
func (l *opPluginLoader) run(ctx context.Context) {
	// Wait for the first request, so a Space whose replay never waits holds
	// no runtime.
	ids, changed := l.getPluginIDs()
	for len(ids) == 0 {
		select {
		case <-ctx.Done():
			return
		case <-changed:
		}
		ids, changed = l.getPluginIDs()
	}

	// Hold the Space plugin runtime, retrying with backoff.
	bo := (&backoff.Backoff{BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL}).Construct()
	_, runtime, runtimeRef, err := l.r.AcquireContentsRuntime(ctx)
	for err != nil {
		l.r.le.WithError(err).Warn("cannot hold the plugin runtime for operation plugins")
		select {
		case <-ctx.Done():
			return
		case <-time.After(bo.NextBackOff()):
		}
		_, runtime, runtimeRef, err = l.r.AcquireContentsRuntime(ctx)
	}
	defer runtimeRef.Release()

	// Demand the requested plugins and follow later requests.
	demand := runtime.DemandPlugins(ids)
	defer demand.Release()
	for {
		l.r.le.WithField("plugin-ids", ids).Debug("running operation plugins")
		select {
		case <-ctx.Done():
			return
		case <-changed:
		}
		ids, changed = l.getPluginIDs()
		demand.SetPluginIDs(ids)
	}
}

// _ is a type assertion
var _ controller.Controller = (*opPluginLoader)(nil)
