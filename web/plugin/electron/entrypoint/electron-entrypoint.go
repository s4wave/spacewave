package main

import (
	"context"
	"os"
	"path"

	plugin_entrypoint "github.com/aperturerobotics/bldr/plugin/entrypoint"
	target_electron "github.com/aperturerobotics/bldr/target/electron"
	bldr_values "github.com/aperturerobotics/bldr/values"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/sirupsen/logrus"
)

// Factories are the factories included in the binary.
var Factories = []plugin_entrypoint.AddFactoryFunc{func(b bus.Bus) []controller.Factory {
	return []controller.Factory{target_electron.NewFactory(b)}
}}

// ConfigSets are the configuration sets to apply on startup.
var ConfigSets = []plugin_entrypoint.BuildConfigSetFunc{BuildConfigSet}

// BuildConfigSet builds the configset to run on startup.
func BuildConfigSet(ctx context.Context, b bus.Bus, le *logrus.Entry) ([]configset.ConfigSet, error) {
	// current working directory is the dist/ directory.
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	electronBin := path.Join(wd, target_electron.GetElectronBinName())
	return []configset.ConfigSet{{
		"electron": configset.NewControllerConfig(1, &target_electron.Config{
			ElectronPath: electronBin,
			WorkdirPath:  wd,
			RendererPath: "./app.asar",
			WebRuntimeId: "electron",
		}),
	}}, nil
}

// main is the main entrypoint.
func main() {
	plugin_entrypoint.Main(Factories, ConfigSets)
}

// _ ensures that at least one reference to bldr_values is present.
var _ bldr_values.EsbuildOutput
