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
		cliImports     map[string]CliImport
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
			cliImports: map[string]CliImport{
				"github.com/example/cli-cmds": {Alias: "example_cli"},
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
			cliImports: map[string]CliImport{
				"github.com/example/beta/cli":  {Alias: "beta_cli"},
				"github.com/example/alpha/cli": {Alias: "alpha_cli"},
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
			cliImports: map[string]CliImport{},
			expected:   expectedCodegenNoBus,
		},
		{
			name:           "empty",
			appName:        "test-empty",
			projectID:      "",
			factoryImports: map[string]FactoryImport{},
			cliImports:     map[string]CliImport{},
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

const expectedCodegenBroker = `package main

import (
	"embed"

	aperture_cli "github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	example_cli "github.com/example/cli-cmds"
	example_factory "github.com/example/factory"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
	yield_policy "github.com/s4wave/spacewave/core/resource/listener/yieldpolicy"
)

// configSetFS contains the embedded configset.
//
//go:embed configset.bin
var configSetFS embed.FS

// listenerBrokers are the shared yield and listener-status brokers wired at
// this composition root into the resource listener controller, the root
// resource controller, and the CLI commands.
type listenerBrokers struct {
	yield  *yield_policy.Broker
	status *resource_listener.StatusBroker
}

// newListenerBrokers constructs the process-shared broker pair.
func newListenerBrokers() *listenerBrokers {
	return &listenerBrokers{yield: yield_policy.NewBroker(), status: resource_listener.NewStatusBroker()}
}

// buildFactories wires the shared listener brokers into every consumer.
func buildFactories(brokers *listenerBrokers) []cli_entrypoint.AddFactoryFunc {
	return []cli_entrypoint.AddFactoryFunc{func(b bus.Bus) []controller.Factory {
		return []controller.Factory{example_factory.NewFactory(b)}
	}, func(b bus.Bus) []controller.Factory {
		return []controller.Factory{resource_listener.NewFactory(b, resource_listener.WithYieldBroker(brokers.yield), resource_listener.WithStatusBroker(brokers.status))}
	}}
}

// configSets are the configuration sets to apply on startup.
var configSets = []cli_entrypoint.BuildConfigSetFunc{cli_entrypoint.ConfigSetFuncFromFS(configSetFS, "configset.bin")}

// buildCliCommands builds the CLI command builders. Each command process
// owns a private broker pair: it never shares listener state with the
// daemon, and its bus's configured resource listener must not displace the
// foreground serve process; serve binds the socket explicitly after
// installing its own handoff guard.
func buildCliCommands(brokers *listenerBrokers) []cli_entrypoint.BuildCommandsFunc {
	return []cli_entrypoint.BuildCommandsFunc{func(getBus func() cli_entrypoint.CliBus) []*aperture_cli.Command {
		handedOff := false
		protectedGetBus := func() cli_entrypoint.CliBus {
			if !handedOff {
				brokers.yield.BeginHandoff("my-app CLI", "")
				handedOff = true
			}
			return getBus()
		}
		return example_cli.NewCliCommands(protectedGetBus, brokers.yield)
	}}
}

// main is the main entrypoint.
func main() {
	brokers := newListenerBrokers()
	cli_entrypoint.Main("my-app", "", buildFactories(brokers), configSets, buildCliCommands(brokers))
}
`

// TestFormatCliEntrypointBrokerWiring verifies the generated entrypoint
// wires the shared listener brokers into broker-consuming factories and
// broker-taking CLI command builders, and imports what that shape needs.
func TestFormatCliEntrypointBrokerWiring(t *testing.T) {
	dat, err := FormatCliEntrypoint("my-app", "", map[string]FactoryImport{
		"github.com/example/factory":                         {Path: "github.com/example/factory", Alias: "example_factory", PassBus: true},
		"github.com/s4wave/spacewave/core/resource/listener": {Path: "github.com/s4wave/spacewave/core/resource/listener", Alias: "resource_listener", PassBus: true},
	}, map[string]CliImport{
		"github.com/example/cli-cmds": {Alias: "example_cli", TakesYieldBroker: true},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	output := strings.TrimSpace(string(dat))
	expected := strings.TrimSpace(expectedCodegenBroker)
	if output != expected {
		dmp := gdiff.New()
		t.Fatalf("generated entrypoint mismatch:\n%s", dmp.DiffPrettyText(dmp.DiffMain(expected, output, false)))
	}
}
