package space_world

import (
	"slices"
	"strings"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
)

// LoadOpPlugins asks the holders of a World engine's plugin runtime to run
// the plugins its replay waits for. The engine keeps one while replay stops
// at an operation whose type those plugins declare, and releases it once a
// handler for the type resolves. It resolves no value.
type LoadOpPlugins interface {
	// Directive indicates LoadOpPlugins is a directive.
	directive.Directive

	// LoadOpPluginsEngineID returns the ID of the waiting World engine.
	LoadOpPluginsEngineID() string
	// LoadOpPluginsPluginIDs returns the sorted plugins to run.
	LoadOpPluginsPluginIDs() []string
}

// loadOpPlugins is an in-memory LoadOpPlugins directive.
type loadOpPlugins struct {
	// engineID is the waiting World engine.
	engineID string
	// pluginIDs are the sorted plugins to run.
	pluginIDs []string
}

// NewLoadOpPlugins constructs a LoadOpPlugins directive for the sorted
// pluginIDs the World engine engineID waits for.
func NewLoadOpPlugins(engineID string, pluginIDs []string) LoadOpPlugins {
	return &loadOpPlugins{engineID: engineID, pluginIDs: pluginIDs}
}

// Validate checks that the directive names a plugin.
func (d *loadOpPlugins) Validate() error {
	if len(d.pluginIDs) == 0 {
		return errors.New("no plugins to load")
	}
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *loadOpPlugins) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// LoadOpPluginsEngineID returns the ID of the waiting World engine.
func (d *loadOpPlugins) LoadOpPluginsEngineID() string {
	return d.engineID
}

// LoadOpPluginsPluginIDs returns the sorted plugins to run.
func (d *loadOpPlugins) LoadOpPluginsPluginIDs() []string {
	return d.pluginIDs
}

// IsEquivalent reports whether other asks the same engine for the same
// plugins.
func (d *loadOpPlugins) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(LoadOpPlugins)
	return ok && od.LoadOpPluginsEngineID() == d.engineID && slices.Equal(od.LoadOpPluginsPluginIDs(), d.pluginIDs)
}

// Superceeds returns false: an equivalent directive is merged instead.
func (d *loadOpPlugins) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
func (d *loadOpPlugins) GetName() string {
	return "LoadOpPlugins"
}

// GetDebugVals returns the directive arguments as strings.
func (d *loadOpPlugins) GetDebugVals() directive.DebugValues {
	return directive.DebugValues{
		"engine-id":  []string{d.engineID},
		"plugin-ids": []string{strings.Join(d.pluginIDs, ",")},
	}
}

// _ is a type assertion
var _ LoadOpPlugins = (*loadOpPlugins)(nil)
