package plugin_builder

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/util/promise"
)

// ControllerConfig is a configuration for a plugin Builder controller.
type ControllerConfig interface {
	// Config is the base config interface.
	config.Config

	// SetPluginBuilderConfig configures the common plugin builder settings.
	SetPluginBuilderConfig(conf *PluginBuilderConfig)
	// SetDisableWatch sets the disable watch field, if applicable.
	SetDisableWatch(disable bool)
}

// Controller is a plugin Builder controller.
//
// The controller builds the plugin and writes the manifest + contents to the
// configured world engine. It should watch for changes and re-build if any
// source files change.
type Controller interface {
	controller.Controller

	// GetResultPromise returns the plugin result promise.
	// Also contains any error that occurs while compiling.
	GetResultPromise() *promise.PromiseContainer[*PluginBuilderResult]
}
