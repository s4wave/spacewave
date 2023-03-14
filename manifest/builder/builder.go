package bldr_manifest_builder

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/util/promise"
)

// ControllerConfig is a configuration for a manifest Builder controller.
type ControllerConfig interface {
	// Config is the base config interface.
	config.Config

	// SetBuilderConfig configures the common builder settings.
	SetBuilderConfig(conf *BuilderConfig)
}

// Controller is a manifest builder controller.
//
// The controller builds and writes the manifest and contents to the configured
// world engine. It should watch for changes and re-build.
type Controller interface {
	controller.Controller

	// GetResultPromise returns the result promise.
	// Also contains any error that occurs while compiling.
	GetResultPromise() *promise.PromiseContainer[*BuilderResult]
}
