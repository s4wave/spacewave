package resource_space

import (
	"context"
	"slices"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/broadcast"
	plugin_space_runtime "github.com/s4wave/spacewave/core/plugin/space/runtime"
	space_world "github.com/s4wave/spacewave/core/space/world"
)

// opPluginLoaderControllerID identifies the operation plugin loader.
const opPluginLoaderControllerID = "spacewave/space/op-plugin-loader"

// opPluginLoader runs the plugins that replay of the Space's World asks for
// with LoadOpPlugins, from the first request until the Space resource is
// released, as a replay demand: a plugin the Space no longer lists still
// loads, but only once the host scheduler admits its manifest root. A plugin
// keeps running after its handler resolves, so later operations of its types
// do not each pause replay and restart it. Each LoadOpPlugins resolves the
// requested plugins whose load the scheduler shows awaiting approval, read
// from the scheduler for as long as the directive lives.
type opPluginLoader struct {
	// r is the Space resource whose plugin runtime runs the plugins.
	r *SpaceResource
	// bcast guards pluginIDs and runtime.
	bcast broadcast.Broadcast
	// pluginIDs is the sorted set of requested plugins.
	pluginIDs []string
	// runtime is the held Space plugin runtime, or nil before run holds it.
	runtime *plugin_space_runtime.Controller
}

// StartOpPluginLoader runs the plugins that replay of the Space's World
// waits for, until the returned release is called. ctx is the mount's
// context: the loader keeps the context's values, which select the same plugin
// runtime an application of this resource acquires, but not the context's
// cancellation.
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
// engine requests, and resolves those awaiting approval.
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
	requested := dir.LoadOpPluginsPluginIDs()
	return []directive.Resolver{directive.NewFuncResolver(func(ctx context.Context, h directive.ResolverHandler) error {
		l.resolveAwaiting(ctx, h, requested)
		return nil
	})}, nil
}

// resolveAwaiting keeps one value on h, the requested plugins awaiting
// approval, until ctx ends: the directive's release ends the watch of the
// running generation's scheduler status.
func (l *opPluginLoader) resolveAwaiting(ctx context.Context, h directive.ResolverHandler, requested []string) {
	var valID uint32
	var hasVal bool
	show := func(ids []string) {
		if hasVal {
			h.RemoveValue(valID)
			hasVal = false
		}
		if len(ids) != 0 {
			valID, hasVal = h.AddValue(space_world.LoadOpPluginsValue(ids))
		}
	}
	for {
		// Wait for run to hold the plugin runtime.
		var runtime *plugin_space_runtime.Controller
		var ch <-chan struct{}
		l.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			runtime, ch = l.runtime, getWaitCh()
		})
		if runtime == nil {
			select {
			case <-ctx.Done():
				return
			case <-ch:
				continue
			}
		}

		// Follow the running generation's scheduler until the generation
		// changes.
		gen, genCh, _ := runtime.GetGeneration()
		if gen == nil {
			show(nil)
			select {
			case <-ctx.Done():
				return
			case <-genCh:
				continue
			}
		}
		genCtx, genCancel := context.WithCancel(ctx)
		go func() {
			select {
			case <-genCh:
				genCancel()
			case <-genCtx.Done():
			}
		}()
		err := gen.GetSpaceController().WatchAwaitingApproval(genCtx, requested, show)
		genCancel()
		if ctx.Err() != nil {
			return
		}
		if err != nil && genCtx.Err() == nil {
			l.r.le.WithError(err).Warn("cannot watch plugin approval for operation plugins")
			select {
			case <-ctx.Done():
				return
			case <-genCh:
			}
		}
	}
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
		//gostyle:timer backoff
		case <-time.After(bo.NextBackOff()):
		}
		_, runtime, runtimeRef, err = l.r.AcquireContentsRuntime(ctx)
	}
	defer runtimeRef.Release()
	l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		l.runtime = runtime
		broadcast()
	})

	// Demand the requested plugins and follow later requests.
	demand := runtime.DemandReplayPlugins(ids)
	defer func() {
		l.r.le.WithField("holder", "op-loader").
			WithField("space-id", l.r.space.GetSharedObjectRef().GetProviderResourceRef().GetId()).
			Debug("released Space plugin demand")
		demand.Release()
	}()
	for {
		l.r.le.WithField("plugin-ids", ids).Debug("running operation plugins")
		select {
		case <-ctx.Done():
			return
		case <-changed:
		}
		next, nextChanged := l.getPluginIDs()
		if !slices.Equal(next, ids) {
			demand.SetPluginIDs(next)
		}
		ids, changed = next, nextChanged
	}
}

// _ is a type assertion
var _ controller.Controller = (*opPluginLoader)(nil)
