package cli_entrypoint

import (
	"context"

	plugin_entrypoint "github.com/aperturerobotics/bldr/plugin/entrypoint"
	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/hydra/world"
	"github.com/sirupsen/logrus"
)

// CliBus is the common interface for CLI bus implementations.
type CliBus interface {
	// GetContext returns the bus context.
	GetContext() context.Context
	// GetBus returns the controller bus.
	GetBus() bus.Bus
	// GetLogger returns the root logger.
	GetLogger() *logrus.Entry
	// GetVolume returns the volume used for state.
	GetVolume() volume.Volume
	// GetWorldEngineID returns the world engine ID.
	GetWorldEngineID() string
	// GetWorldEngine returns the world engine instance.
	GetWorldEngine() world.Engine
	// GetWorldState returns the world state instance.
	GetWorldState() world.WorldState
	// GetPluginHostObjectKey returns the plugin host object key.
	GetPluginHostObjectKey() string
	// Release releases all resources held by the bus.
	Release()
}

// AddFactoryFunc is a callback to add a factory.
type AddFactoryFunc = plugin_entrypoint.AddFactoryFunc

// BuildConfigSetFunc is a function to build a list of ConfigSet to apply.
type BuildConfigSetFunc = plugin_entrypoint.BuildConfigSetFunc

// BuildCommandsFunc is a function to build CLI commands.
type BuildCommandsFunc func(getBus func() CliBus) []*cli.Command

// ConfigSetFuncFromFS builds a ConfigSetFunc which parses a file in a FS as a ConfigSet.
var ConfigSetFuncFromFS = plugin_entrypoint.ConfigSetFuncFromFS
