package plugin_builder

import "github.com/aperturerobotics/controllerbus/config"

// Config is a configuration for a plugin Builder.
//
// The controller will build the plugin & write the manifest & contents to the
// configured world engine. It should watch for changes and re-build if any
// source files change.
type Config interface {
	// Config is the base config interface.
	config.Config

	// SetPluginId configures the plugin ID to build.
	SetPluginId(pluginID string)
	// SetEngineId configures the world engine ID to attach to.
	SetEngineId(worldEngineID string)
	// SetPluginHostKey configures the plugin host object key.
	SetPluginHostKey(pluginHostObjKey string)
	// SetPlatformId configures the platform ID to compile for.
	SetPlatformId(platformID string)
}
