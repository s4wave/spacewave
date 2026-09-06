//go:build !js && !wasip1 && !wasm

package plugin_host_default

// materializerPluginID returns the plugin ID to route manifest materialization
// through. Native desktop platforms materialize manifests directly, so this
// returns empty.
func materializerPluginID() string {
	return ""
}
