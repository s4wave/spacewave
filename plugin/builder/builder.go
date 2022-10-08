package plugin_builder

import (
	"github.com/aperturerobotics/controllerbus/config"
)

// ControllerConfig is a configuration for a plugin Builder controller.
//
// The controller will build the plugin & write the manifest & contents to the
// configured world engine. It should watch for changes and re-build if any
// source files change.
type ControllerConfig interface {
	// Config is the base config interface.
	config.Config

	// SetPluginBuilderConfig configures the common plugin builder settings.
	SetPluginBuilderConfig(conf *PluginBuilderConfig)
}
