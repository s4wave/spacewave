//go:build js || wasip1 || wasm

package plugin_host_default

import (
	bldr_manifest_materializer "github.com/s4wave/spacewave/bldr/manifest/materializer"
)

// materializerPluginID returns the plugin ID to route manifest materialization
// through on browser platforms. Native platforms execute copies directly.
func materializerPluginID() string {
	return bldr_manifest_materializer.PluginID
}
