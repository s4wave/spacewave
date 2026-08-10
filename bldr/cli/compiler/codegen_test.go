//go:build !js

package bldr_cli_compiler

import (
	"os"
	"os/exec"
	"path/filepath"
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

func TestAllocateGoImportsCompilesKeywordAndDuplicateFactoryPackages(t *testing.T) {
	factories, cliImports := allocateGoImports(
		map[string]FactoryImport{
			"example.invalid/generated/first/dup":  {Alias: "dup", PassBus: true},
			"example.invalid/generated/second/dup": {Alias: "dup", PassBus: true},
		},
		[]string{
			"example.invalid/generated/bus",
			"example.invalid/generated/cliCommands",
			"example.invalid/generated/configSets",
			"example.invalid/generated/first/dup",
		},
	)
	if got := factories["example.invalid/generated/first/dup"].Alias; got != "dup" {
		t.Fatalf("first factory alias = %q, want dup", got)
	}
	if got := factories["example.invalid/generated/second/dup"].Alias; got != "dup_2" {
		t.Fatalf("second factory alias = %q, want dup_2", got)
	}
	if got := cliImports["example.invalid/generated/bus"]; got != "bus_2" {
		t.Fatalf("keyword CLI alias = %q, want bus_2", got)
	}
	if got := cliImports["example.invalid/generated/first/dup"]; got != "dup" {
		t.Fatalf("shared package alias = %q, want dup", got)
	}
	if got := cliImports["example.invalid/generated/configSets"]; got != "configSets_2" {
		t.Fatalf("configSets CLI alias = %q, want configSets_2", got)
	}
	if got := cliImports["example.invalid/generated/cliCommands"]; got != "cliCommands_2" {
		t.Fatalf("cliCommands CLI alias = %q, want cliCommands_2", got)
	}

	src, err := FormatCliEntrypoint("imports", "", factories, cliImports)
	if err != nil {
		t.Fatal(err)
	}
	compileGeneratedEntrypoint(t, src)
}

func compileGeneratedEntrypoint(t *testing.T, src []byte) {
	t.Helper()
	root := t.TempDir()
	write := func(name, data string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", `module generated.test

go 1.24

require (
	github.com/aperturerobotics/controllerbus v0.0.0
	github.com/s4wave/spacewave v0.0.0
	example.invalid/generated v0.0.0
)

replace github.com/aperturerobotics/controllerbus => ./controllerbus
replace github.com/s4wave/spacewave => ./spacewave
replace example.invalid/generated => ./generated
`)
	write("main.go", string(src))
	write("configset.bin", "")
	write("controllerbus/go.mod", "module github.com/aperturerobotics/controllerbus\n\ngo 1.24\n")
	write("controllerbus/bus/bus.go", "package bus\ntype Bus interface{}\n")
	write("controllerbus/controller/controller.go", "package controller\ntype Factory interface{}\n")
	write("spacewave/go.mod", `module github.com/s4wave/spacewave

go 1.24

require github.com/aperturerobotics/controllerbus v0.0.0
replace github.com/aperturerobotics/controllerbus => ../controllerbus
`)
	write("spacewave/bldr/cli/entrypoint/entrypoint.go", `package cli_entrypoint

import (
	"embed"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
)

type AddFactoryFunc func(bus.Bus) []controller.Factory
type BuildConfigSetFunc func()
type BuildCommandsFunc func()
func ConfigSetFuncFromFS(embed.FS, string) BuildConfigSetFunc { return func() {} }
func Main(string, string, []AddFactoryFunc, []BuildConfigSetFunc, []BuildCommandsFunc) {}
`)
	write("generated/go.mod", `module example.invalid/generated

go 1.24

require github.com/aperturerobotics/controllerbus v0.0.0
replace github.com/aperturerobotics/controllerbus => ../controllerbus
`)
	factory := `package dup
import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
)
func NewFactory(bus.Bus) controller.Factory { return nil }
func NewCliCommands() {}
`
	write("generated/first/dup/dup.go", factory)
	write("generated/second/dup/dup.go", factory)
	write("generated/bus/bus.go", "package bus\nfunc NewCliCommands() {}\n")
	write("generated/configSets/config-sets.go", "package configSets\nfunc NewCliCommands() {}\n")
	write("generated/cliCommands/cli-commands.go", "package cliCommands\nfunc NewCliCommands() {}\n")

	cmd := exec.Command("go", "test", ".")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated entrypoint did not compile: %v\n%s", err, output)
	}
}
