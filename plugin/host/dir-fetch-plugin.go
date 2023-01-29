package plugin_host

import (
	"context"
	"errors"
	"time"

	"github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// FetchPlugin is a directive to fetch a plugin manifest to storage.
type FetchPlugin interface {
	// Directive indicates FetchPlugin is a directive.
	directive.Directive

	// FetchPluginID returns the plugin ID to fetch.
	// Cannot be empty.
	FetchPluginID() string
}

// FetchPluginValue is the result type for FetchPlugin.
// Multiple results may be pushed to the directive.
type FetchPluginValue = *plugin.FetchPluginResponse

// fetchPlugin implements FetchPlugin
type fetchPlugin struct {
	pluginID string
}

// NewFetchPlugin constructs a new FetchPlugin directive.
func NewFetchPlugin(pluginID string) FetchPlugin {
	return &fetchPlugin{pluginID: pluginID}
}

// ExFetchPlugin executes the FetchPlugin directive.
func ExFetchPlugin(
	ctx context.Context,
	b bus.Bus,
	pluginID string,
	returnIfIdle bool,
) (FetchPluginValue, error) {
	av, _, avRef, err := bus.ExecOneOff(ctx, b, NewFetchPlugin(pluginID), returnIfIdle, nil)
	if err != nil {
		return nil, err
	}
	if avRef == nil {
		return nil, errors.New("fetch plugin returned empty result")
	}
	avRef.Release()
	val, ok := av.GetValue().(FetchPluginValue)
	if !ok {
		return nil, errors.New("fetch plugin directive returned invalid result type")
	}
	return val, nil
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *fetchPlugin) Validate() error {
	if d.pluginID == "" {
		return plugin.ErrEmptyPluginID
	}

	return nil
}

// GetValueFetchPluginOptions returns options relating to value handling.
func (d *fetchPlugin) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		// UnrefDisposeDur is the duration to wait to dispose a directive after all
		// references have been released.
		UnrefDisposeDur: time.Second * 3,
	}
}

// FetchPluginID returns the plugin ID.
func (d *fetchPlugin) FetchPluginID() string {
	return d.pluginID
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *fetchPlugin) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(FetchPlugin)
	if !ok {
		return false
	}

	if d.FetchPluginID() != od.FetchPluginID() {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *fetchPlugin) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *fetchPlugin) GetName() string {
	return "FetchPlugin"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *fetchPlugin) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["plugin-id"] = []string{d.FetchPluginID()}
	return vals
}

// _ is a type assertion
var _ FetchPlugin = ((*fetchPlugin)(nil))
