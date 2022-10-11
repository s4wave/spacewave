package plugin_host

import (
	"context"
	"errors"

	"github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
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
type LoadPluginValue = *PluginStateSnapshot

// loadPlugin implements LoadPlugin
type loadPlugin struct {
	pluginID string
}

// NewLoadPlugin constructs a new LoadPlugin directive.
func NewLoadPlugin(pluginID string) LoadPlugin {
	return &loadPlugin{pluginID: pluginID}
}

// ExLoadPlugin executes the LoadPlugin directive.
func ExLoadPlugin(
	ctx context.Context,
	b bus.Bus,
	pluginID string,
	cb func(LoadPluginValue) error,
) error {
	avCh, avRef, err := bus.ExecOneOffWatchCh(b, NewLoadPlugin(pluginID))
	if err != nil {
		return err
	}
	defer avRef.Release()
	for {
		select {
		case <-ctx.Done():
			return context.Canceled
		case av, ok := <-avCh:
			if !ok {
				return context.Canceled
			}
			val, valOk := av.GetValue().(LoadPluginValue)
			if !valOk {
				return errors.New("load plugin directive returned invalid result")
			}
			if cb != nil {
				if err := cb(val); err != nil {
					return err
				}
			}
		}
	}
}

// ExLoadPluginWaitClient calls LoadPlugin and returns the rpc client to be set.
// if returnIfIdle is set, returns nil, nil, nil if the directive becomes idle.
func ExPluginLoadWaitClient(
	ctx context.Context,
	b bus.Bus,
	pluginID string,
	returnIfIdle bool,
) (srpc.Client, directive.Reference, error) {
	v, dirRef, err := bus.ExecWaitValue(
		ctx,
		b,
		NewLoadPlugin(pluginID),
		returnIfIdle,
		func(val LoadPluginValue) (bool, error) {
			if val != nil && val.RpcClient != nil {
				return true, nil
			}
			return false, nil
		},
	)
	if err != nil {
		return nil, nil, err
	}
	if v == nil {
		return nil, nil, nil
	}
	return v.RpcClient, dirRef, nil
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

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *loadPlugin) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
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
var _ LoadPlugin = ((*loadPlugin)(nil))
