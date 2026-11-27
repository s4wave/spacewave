//go:build !js

package plugin_host_process

import "time"

// PluginPackageStatus describes native dist materialization state for one
// plugin. It deliberately excludes plugin state-dir contents.
type PluginPackageStatus struct {
	// PluginID is the plugin the status describes.
	PluginID string
	// DistDir is the plugin's dist directory.
	DistDir string
	// Materialized is set when the last sync wrote the plugin's checkout.
	Materialized bool
	// LastAction names the last action: "sync".
	LastAction string
	// LastError is the last action's error, or empty when it succeeded.
	LastError string
	// UpdatedAt is when the last action finished, in UTC.
	UpdatedAt time.Time
}
