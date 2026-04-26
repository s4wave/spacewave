//go:build !js

package bldr_cli_compiler

import (
	"strings"
	"testing"

	gdiff "github.com/sergi/go-diff/diffmatchpatch"
)

const expectedCodegenWithImports = `package main

import (
	"embed"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	example_cli "github.com/example/cli-cmds"
	example_factory "github.com/example/factory"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
)

// configSetFS contains the embedded configset.
//
//go:embed configset.bin
var configSetFS embed.FS

// factories are the factories included in the binary.
var factories = []cli_entrypoint.AddFactoryFunc{func(b bus.Bus) []controller.Factory {
	return []controller.Factory{example_factory.NewFactory(b)}
}}

// configSets are the configuration sets to apply on startup.
var configSets = []cli_entrypoint.BuildConfigSetFunc{cli_entrypoint.ConfigSetFuncFromFS(configSetFS, "configset.bin")}

// cliCommands are the CLI command builders.
var cliCommands = []cli_entrypoint.BuildCommandsFunc{example_cli.NewCliCommands}

// main is the main entrypoint.
func main() { cli_entrypoint.Main("my-app", "", factories, configSets, cliCommands) }
`

const expectedCodegenMultiple = `package main

import (
	"embed"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	alpha_cli "github.com/example/alpha/cli"
	alpha_factory "github.com/example/alpha/factory"
	beta_cli "github.com/example/beta/cli"
	beta_factory "github.com/example/beta/factory"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
)

// configSetFS contains the embedded configset.
//
//go:embed configset.bin
var configSetFS embed.FS

// factories are the factories included in the binary.
var factories = []cli_entrypoint.AddFactoryFunc{func(b bus.Bus) []controller.Factory {
	return []controller.Factory{alpha_factory.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{beta_factory.NewFactory(b)}
}}

// configSets are the configuration sets to apply on startup.
var configSets = []cli_entrypoint.BuildConfigSetFunc{cli_entrypoint.ConfigSetFuncFromFS(configSetFS, "configset.bin")}

// cliCommands are the CLI command builders.
var cliCommands = []cli_entrypoint.BuildCommandsFunc{alpha_cli.NewCliCommands, beta_cli.NewCliCommands}

// main is the main entrypoint.
func main() { cli_entrypoint.Main("multi-app", "", factories, configSets, cliCommands) }
`

const expectedCodegenNoBus = `package main

import (
	"embed"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	example_factory "github.com/example/factory"
	no_bus_fc "github.com/example/no-bus/fc"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
)

// configSetFS contains the embedded configset.
//
//go:embed configset.bin
var configSetFS embed.FS

// factories are the factories included in the binary.
var factories = []cli_entrypoint.AddFactoryFunc{func(b bus.Bus) []controller.Factory {
	return []controller.Factory{example_factory.NewFactory(b)}
}, func(b bus.Bus) []controller.Factory {
	return []controller.Factory{no_bus_fc.NewFactory()}
}}

// configSets are the configuration sets to apply on startup.
var configSets = []cli_entrypoint.BuildConfigSetFunc{cli_entrypoint.ConfigSetFuncFromFS(configSetFS, "configset.bin")}

// cliCommands are the CLI command builders.
var cliCommands = []cli_entrypoint.BuildCommandsFunc{}

// main is the main entrypoint.
func main() { cli_entrypoint.Main("no-bus-app", "", factories, configSets, cliCommands) }
`

const expectedCodegenEmpty = `package main

import (
	"embed"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
)

// configSetFS contains the embedded configset.
//
//go:embed configset.bin
var configSetFS embed.FS

// factories are the factories included in the binary.
var factories = []cli_entrypoint.AddFactoryFunc{}

// configSets are the configuration sets to apply on startup.
var configSets = []cli_entrypoint.BuildConfigSetFunc{cli_entrypoint.ConfigSetFuncFromFS(configSetFS, "configset.bin")}

// cliCommands are the CLI command builders.
var cliCommands = []cli_entrypoint.BuildCommandsFunc{}

// main is the main entrypoint.
func main() { cli_entrypoint.Main("test-empty", "", factories, configSets, cliCommands) }
`

func TestFormatCliEntrypoint(t *testing.T) {
	type testcase struct {
		name           string
		appName        string
		projectID      string
		factoryImports map[string]FactoryImport
		cliImports     map[string]string
		expected       string
	}
	tests := []*testcase{
		{
			name:      "with imports",
			appName:   "my-app",
			projectID: "",
			factoryImports: map[string]FactoryImport{
				"github.com/example/factory": {Alias: "example_factory", PassBus: true},
			},
			cliImports: map[string]string{
				"github.com/example/cli-cmds": "example_cli",
			},
			expected: expectedCodegenWithImports,
		},
		{
			name:      "multiple",
			appName:   "multi-app",
			projectID: "",
			factoryImports: map[string]FactoryImport{
				"github.com/example/beta/factory":  {Alias: "beta_factory", PassBus: true},
				"github.com/example/alpha/factory": {Alias: "alpha_factory", PassBus: true},
			},
			cliImports: map[string]string{
				"github.com/example/beta/cli":  "beta_cli",
				"github.com/example/alpha/cli": "alpha_cli",
			},
			expected: expectedCodegenMultiple,
		},
		{
			name:      "no-bus",
			appName:   "no-bus-app",
			projectID: "",
			factoryImports: map[string]FactoryImport{
				"github.com/example/factory":   {Alias: "example_factory", PassBus: true},
				"github.com/example/no-bus/fc": {Alias: "no_bus_fc", PassBus: false},
			},
			cliImports: map[string]string{},
			expected:   expectedCodegenNoBus,
		},
		{
			name:           "empty",
			appName:        "test-empty",
			projectID:      "",
			factoryImports: map[string]FactoryImport{},
			cliImports:     map[string]string{},
			expected:       expectedCodegenEmpty,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dat, err := FormatCliEntrypoint(tc.appName, tc.projectID, tc.factoryImports, tc.cliImports)
			if err != nil {
				t.Fatal(err.Error())
			}
			output := strings.TrimSpace(string(dat))
			expected := strings.TrimSpace(tc.expected)
			if output != expected {
				t.Logf("expected:\n%s", expected)
				t.Logf("actual:\n%s", output)
				dmp := gdiff.New()
				diffs := dmp.DiffMain(expected, output, false)
				t.Fatal(dmp.DiffPrettyText(diffs))
			}
		})
	}
}
