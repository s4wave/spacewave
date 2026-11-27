// Package bldr_plugin defines how a caller asks for a running plugin and what it
// gets back.
//
// LoadPlugin is the directive that expresses standing demand for a plugin ID.
// With an empty instance key all callers share one instance. A non-empty key
// makes (plugin ID, instance key) its own instance. By default the scheduler
// follows the newest eligible Manifest. LoadPluginManifestRoot pins an exact
// Manifest and never falls back when it is missing, and LoadPluginManifests
// selects an installation and its recovery artifacts, approved by
// LoadPluginPeerID. The directive's value is a RunningPlugin, which exposes the
// plugin's RPC client for one execution generation. A caller holds the
// directive reference for the whole of its call, and a call made before the
// plugin is ready waits until the plugin is ready or the caller cancels. A
// replaced or restarted plugin is a new generation that must register its
// capabilities again.
//
// The host, scheduler and transports accept plugin IDs from their callers and
// recognize no product plugin by name.
package bldr_plugin

import (
	"net/url"
	"strings"

	"github.com/s4wave/spacewave/bldr/plugin/pluginid"
)

// ValidatePluginID validates a plugin ID.
func ValidatePluginID(id string, allowEmpty bool) error {
	return pluginid.Validate(id, allowEmpty)
}

// BuildPluginRpcComponentID addresses a plugin binding and optional exact executable.
func BuildPluginRpcComponentID(pluginID, instanceKey, manifestRoot string) string {
	// Start the component ID with the plugin ID.
	componentID := pluginID

	// Append the escaped instance key when present.
	if instanceKey != "" {
		componentID += "/" + url.PathEscape(instanceKey)
	}

	// Append the manifest root as a query parameter when present.
	if manifestRoot != "" {
		componentID += "?manifest=" + url.QueryEscape(manifestRoot)
	}

	return componentID
}

// ParsePluginRpcComponentID keeps executable identity separate from instance routing.
func ParsePluginRpcComponentID(componentID string) (pluginID, instanceKey, manifestRoot string, err error) {
	// Split the component ID into path and query, then unescape the instance key.
	path, query, _ := strings.Cut(componentID, "?")
	pluginID, instanceKey, _ = strings.Cut(path, "/")
	instanceKey, err = url.PathUnescape(instanceKey)
	if err != nil {
		return
	}

	// Parse the query parameters and read the manifest root.
	values, err := url.ParseQuery(query)
	if err != nil {
		return
	}
	manifestRoot = values.Get("manifest")
	return
}

// Validate validates the LoadPlugin request.
func (r *LoadPluginRequest) Validate() error {
	if err := ValidatePluginID(r.GetPluginId(), false); err != nil {
		return err
	}
	return nil
}
