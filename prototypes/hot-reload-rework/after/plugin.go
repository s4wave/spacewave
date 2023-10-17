package main

import (
	"embed"

	bldr_example "github.com/aperturerobotics/bldr/example"
	plugin_entrypoint "github.com/aperturerobotics/bldr/plugin/entrypoint"

	"os"
	"strings"

	bldr_values "github.com/aperturerobotics/bldr/values"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/sirupsen/logrus"
)

// StaticFS contains embedded static assets.
//
//go:embed config-set.bin
var StaticFS embed.FS

// PluginStartInfo contains the b58 encoded startup information.
var PluginStartInfo = strings.TrimSpace(os.Getenv("BLDR_PLUGIN_START_INFO"))

// PluginMeta contains the b58 encoded plugin metadata.
var PluginMeta = "2QyfLEpASuwumfXX3VpSKuUa5DXygfghqR4rce1tLJUBAJJXRAj"

// LogLevel is the default program log level.
var LogLevel = logrus.DebugLevel

// Factories are the factories included in the binary.
var Factories = []plugin_entrypoint.AddFactoryFunc{func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_example.NewFactory(b)}
}}

// ConfigSets are the configuration sets to apply on startup.
var ConfigSets = []plugin_entrypoint.BuildConfigSetFunc{plugin_entrypoint.ConfigSetFuncFromFS(StaticFS, "config-set.bin")}

// init sets variables at init time
func init() {
	devInfo, err := plugin_entrypoint.PluginDevInfoFromFile("dev-info.bin")
	if err != nil {
		panic(err)
	}
	bldr_example.AssetPath = devInfo.GetPluginDevVars()["bldr_example.AssetPath"].GetAssetPath()
	bldr_example.ExampleEntrypoint = devInfo.GetPluginDevVars()["bldr_example.ExampleEntrypoint"].GetEsbuildOutputValue()
}

// main is the main entrypoint.
func main() {
	plugin_entrypoint.Main(PluginStartInfo, PluginMeta, LogLevel, Factories, ConfigSets)
}

// _ ensures that at least one reference to bldr_values is present.
var _ bldr_values.EsbuildOutput
