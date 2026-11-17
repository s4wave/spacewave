package device_flowgraph

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/backoff"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	"github.com/sirupsen/logrus"
)

// PluginLoaderControllerID identifies the node type plugin loader.
const PluginLoaderControllerID = "spacewave/device/flowgraph/plugin-loader"

// HoldNodeTypePlugins runs the plugins that hint at typeID until ctx ends,
// returning nil then, or returns an error when the hold fails.
type HoldNodeTypePlugins func(ctx context.Context, typeID string) error

// PluginLoader starts the plugins that supply a node type while a lookup of
// that type for its World runs on the bus. The Reconciler holds a lookup for each type a
// placed and allowed node uses, so each such node's plugin starts on demand
// and stops after the last node using its types is removed. The loader adds
// no value: the started plugin's registration resolves the lookup.
type PluginLoader struct {
	// le reports failed holds.
	le *logrus.Entry
	// engineID is the ID of the World engine whose plugins the loader runs.
	engineID string
	// hold runs the plugins of one node type.
	hold HoldNodeTypePlugins
}

// NewPluginLoader constructs a PluginLoader that runs the plugins of the World
// engineID through hold.
func NewPluginLoader(le *logrus.Entry, engineID string, hold HoldNodeTypePlugins) *PluginLoader {
	return &PluginLoader{le: le, engineID: engineID, hold: hold}
}

// GetControllerInfo returns information about the controller.
func (l *PluginLoader) GetControllerInfo() *controller.Info {
	return controller.NewInfo(PluginLoaderControllerID, controller.MustParseVersion("0.0.1"), "Flowgraph node type plugin loader")
}

// Execute returns immediately, since the loader only handles directives.
func (l *PluginLoader) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective holds the plugins of each node type looked up for its World.
func (l *PluginLoader) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir, ok := di.GetDirective().(s4wave_flowgraph.LookupFlowgraphNodeType)
	if !ok || dir.LookupFlowgraphNodeTypeEngineID() != l.engineID {
		return nil, nil
	}
	return directive.R(&pluginLoadResolver{l: l, typeID: dir.LookupFlowgraphNodeTypeID()}, nil)
}

// Close releases nothing.
func (l *PluginLoader) Close() error {
	return nil
}

// pluginLoadResolver holds the plugins of one node type while its lookup runs.
type pluginLoadResolver struct {
	// l is the loader that holds the plugins.
	l *PluginLoader
	// typeID is the looked-up node type.
	typeID string
}

// Resolve holds the plugins until the lookup ends, retrying a failed hold. It
// marks itself idle at once, since it never adds a value.
func (r *pluginLoadResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	handler.MarkIdle(true)
	bo := (&backoff.Backoff{BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL}).Construct()
	for {
		// Hold until the lookup ends or the hold fails.
		err := r.l.hold(ctx, r.typeID)
		if ctx.Err() != nil {
			return nil
		}

		// Retry after a delay, so a lost Space connection recovers.
		delay := bo.NextBackOff()
		r.l.le.WithError(err).WithField("node-type-id", r.typeID).Warn("node type plugin hold failed")
		select {
		case <-ctx.Done():
			return nil
		//gostyle:timer backoff
		case <-time.After(delay):
		}
	}
}

// _ is a type assertion.
var (
	_ controller.Controller = (*PluginLoader)(nil)
	_ directive.Resolver    = (*pluginLoadResolver)(nil)
)
