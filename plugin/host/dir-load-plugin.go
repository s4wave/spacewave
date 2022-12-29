package plugin_host

import (
	"context"

	"github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
)

// LoadPlugin is a directive to execute a plugin.
type LoadPlugin interface {
	// Directive indicates LoadPlugin is a directive.
	directive.Directive

	// LoadPluginID returns the plugin ID to load.
	// Cannot be empty.
	LoadPluginID() string
}

// LoadPluginValue is the result type for LoadPlugin.
// Multiple results may be pushed to the directive.
type LoadPluginValue = RunningPlugin

// RunningPlugin is the interface exposed to callers of LoadPlugin.
type RunningPlugin interface {
	// GetRpcClientCtr returns the rpc client container.
	// The plugin RPC client will be set when the plugin becomes ready.
	GetRpcClientCtr() *ccontainer.CContainer[*srpc.Client]
}

// loadPlugin implements LoadPlugin
type loadPlugin struct {
	pluginID string
}

// NewLoadPlugin constructs a new LoadPlugin directive.
func NewLoadPlugin(pluginID string) LoadPlugin {
	return &loadPlugin{pluginID: pluginID}
}

// ExLoadPlugin executes the LoadPlugin directive.
//
// if returnIfIdle=true and the directive becomes idle, returns nil, nil, nil
func ExLoadPlugin(
	ctx context.Context,
	b bus.Bus,
	returnIfIdle bool,
	pluginID string,
	valDisposeCallback func(),
) (RunningPlugin, directive.Reference, error) {
	return bus.ExecWaitValue[RunningPlugin](
		ctx,
		b,
		NewLoadPlugin(pluginID),
		returnIfIdle,
		valDisposeCallback,
		nil,
	)
}

// ExLoadPluginWaitClient calls LoadPlugin and returns the rpc client to be set.
// if returnIfIdle is set, returns nil, nil, nil if the directive becomes idle.
func ExPluginLoadWaitClient(
	ctx context.Context,
	b bus.Bus,
	pluginID string,
) (srpc.Client, directive.Reference, error) {
	var prevRpRef directive.Reference
	for {
		select {
		case <-ctx.Done():
			if prevRpRef != nil {
				prevRpRef.Release()
			}
			return nil, nil, context.Canceled
		default:
		}
		waitCtx, waitCtxCancel := context.WithCancel(ctx)
		var err error
		rp, rpRef, err := ExLoadPlugin(ctx, b, false, pluginID, waitCtxCancel)
		if prevRpRef != nil {
			prevRpRef.Release()
		}
		prevRpRef = rpRef
		if err != nil {
			waitCtxCancel()
			return nil, nil, err
		}
		// WaitValue waits for a non-nil client value.
		clientCtr := rp.GetRpcClientCtr()
		client, err := clientCtr.WaitValue(waitCtx, nil)
		if err != nil {
			if err == context.Canceled {
				continue
			}
			rpRef.Release()
			return nil, nil, err
		}
		return *client, rpRef, nil
	}
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *loadPlugin) Validate() error {
	if d.pluginID == "" {
		return plugin.ErrEmptyPluginID
	}

	return nil
}

// GetValueLoadPluginOptions returns options relating to value handling.
func (d *loadPlugin) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// LoadPluginID returns the plugin ID.
func (d *loadPlugin) LoadPluginID() string {
	return d.pluginID
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *loadPlugin) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LoadPlugin)
	if !ok {
		return false
	}

	if d.LoadPluginID() != od.LoadPluginID() {
		return false
	}

	return true
}

// GetName returns the directive's type name.
func (d *loadPlugin) GetName() string {
	return "LoadPlugin"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *loadPlugin) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["plugin-id"] = []string{d.LoadPluginID()}
	return vals
}

// _ is a type assertion
var (
	_ LoadPlugin                   = ((*loadPlugin)(nil))
	_ directive.DirectiveWithEquiv = ((*loadPlugin)(nil))
)
