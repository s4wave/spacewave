package plugin_list

import (
	"context"
	"errors"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	plugin_approval "github.com/s4wave/spacewave/core/plugin/approval"
)

// ListAvailablePlugins is a directive to list available plugins for a Space.
//
// Value type: *AvailablePluginList
type ListAvailablePlugins interface {
	// Directive indicates ListAvailablePlugins is a directive.
	directive.Directive

	// ListAvailablePluginsSpaceID returns the Space ID.
	ListAvailablePluginsSpaceID() string
}

// AvailablePlugin describes a plugin's status in a Space.
type AvailablePlugin struct {
	// ManifestID is the manifest identifier.
	ManifestID string
	// Approved indicates the approval state.
	Approved plugin_approval.PluginApprovalState
	// Loaded indicates if the plugin is currently running.
	Loaded bool
	// ManifestInfo contains metadata from the plugin manifest.
	ManifestInfo *ManifestInfo
}

// AvailablePluginList wraps a slice of AvailablePlugin for use as a directive value.
type AvailablePluginList struct {
	// Plugins is the list of available plugins.
	Plugins []*AvailablePlugin
}

// ListAvailablePluginsValue is the result type for ListAvailablePlugins.
type ListAvailablePluginsValue = *AvailablePluginList

// ErrEmptySpaceID is returned when the space ID is empty.
var ErrEmptySpaceID = errors.New("space id cannot be empty")

// ExListAvailablePlugins executes the ListAvailablePlugins directive on the bus.
//
// Returns the list of available plugins for the given space.
func ExListAvailablePlugins(
	ctx context.Context,
	b bus.Bus,
	spaceID string,
) (*AvailablePluginList, directive.Reference, error) {
	av, _, ref, err := bus.ExecOneOffTyped[ListAvailablePluginsValue](
		ctx,
		b,
		NewListAvailablePlugins(spaceID),
		bus.ReturnWhenIdle(),
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	if av == nil {
		if ref != nil {
			ref.Release()
		}
		return nil, nil, nil
	}
	return av.GetValue(), ref, nil
}

// listAvailablePlugins implements ListAvailablePlugins.
type listAvailablePlugins struct {
	spaceID string
}

// NewListAvailablePlugins constructs a new ListAvailablePlugins directive.
func NewListAvailablePlugins(spaceID string) ListAvailablePlugins {
	return &listAvailablePlugins{
		spaceID: spaceID,
	}
}

// Validate validates the directive.
func (d *listAvailablePlugins) Validate() error {
	if d.spaceID == "" {
		return ErrEmptySpaceID
	}
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *listAvailablePlugins) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		UnrefDisposeDur:            time.Millisecond * 100,
		UnrefDisposeEmptyImmediate: true,
	}
}

// ListAvailablePluginsSpaceID returns the Space ID.
func (d *listAvailablePlugins) ListAvailablePluginsSpaceID() string {
	return d.spaceID
}

// IsEquivalent checks if the other directive is equivalent.
func (d *listAvailablePlugins) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(ListAvailablePlugins)
	if !ok {
		return false
	}
	return d.ListAvailablePluginsSpaceID() == od.ListAvailablePluginsSpaceID()
}

// Superceeds checks if the directive overrides another.
func (d *listAvailablePlugins) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
func (d *listAvailablePlugins) GetName() string {
	return "ListAvailablePlugins"
}

// GetDebugVals returns the directive arguments stringified.
func (d *listAvailablePlugins) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if d.spaceID != "" {
		vals["space-id"] = []string{d.spaceID}
	}
	return vals
}

// _ is a type assertion
var _ ListAvailablePlugins = ((*listAvailablePlugins)(nil))
