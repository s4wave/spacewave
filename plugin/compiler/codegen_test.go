package bldr_plugin_compiler

import (
	"context"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	gdiff "github.com/sergi/go-diff/diffmatchpatch"
	"github.com/sirupsen/logrus"
	"golang.org/x/tools/imports"
)

const expectedCodegen = `package main

import (
	"embed"
	"os"
	"strings"

	bldr_example "github.com/aperturerobotics/bldr/example"
	plugin_entrypoint "github.com/aperturerobotics/bldr/plugin/entrypoint"
	bldr_values "github.com/aperturerobotics/bldr/values"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	"github.com/sirupsen/logrus"
)

// StaticFS contains embedded static assets.
//
//go:embed config-set.bin
var StaticFS embed.FS

// PluginStartInfo contains the b58 encoded startup information.
var PluginStartInfo = strings.TrimSpace(os.Getenv("BLDR_PLUGIN_START_INFO"))

// PluginMeta contains the b58 encoded plugin metadata.
var PluginMeta = "8j7eujJNz6qYqGbGLPN2CjBPAtpgeC7tBgcj2dMrNafy5U5nPjF8K4e3SBpgNs5Hpg3"

// LogLevel is the default program log level.
var LogLevel = logrus.DebugLevel

// Factories are the factories included in the binary.
var Factories = []plugin_entrypoint.AddFactoryFunc{func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_example.NewFactory(b), boilerplate_controller.NewFactory(b)}
}}

// ConfigSets are the configuration sets to apply on startup.
var ConfigSets = []plugin_entrypoint.BuildConfigSetFunc{plugin_entrypoint.ConfigSetFuncFromFS(StaticFS, "config-set.bin")}

// init sets variables at init time
func init() {
	bldr_example.ExampleScriptPath = "/path/to/script.js"
}

// main is the main entrypoint.
func main() {
	plugin_entrypoint.Main(PluginStartInfo, PluginMeta, LogLevel, Factories, ConfigSets)
}

// _ ensures that at least one reference to bldr_values is present.
var _ bldr_values.EsbuildOutput
`

func TestCodegen(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	packagePaths := []string{
		"github.com/aperturerobotics/controllerbus/example/boilerplate/controller",
		"github.com/aperturerobotics/bldr/example",
	}
	workDir, _ := os.Getwd()
	workDir = filepath.Join(workDir, "../..")
	an, err := AnalyzePackages(ctx, le, workDir, packagePaths)
	if err != nil {
		t.Fatal(err.Error())
	}
	pluginMeta := bldr_plugin.NewPluginMeta("test-project", "test-plugin", "native/linux/amd64")
	genFile, err := GeneratePluginWrapper(
		le,
		an,
		pluginMeta,
		[]string{"config-set.bin"},
		[]*GoVarDef{NewGoVarDef(
			"github.com/aperturerobotics/bldr/example",
			"ExampleScriptPath",
			&ast.BasicLit{
				Kind:  token.STRING,
				Value: `"/path/to/script.js"`,
			},
		)},
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	formatDat, err := FormatFile(genFile)
	if err != nil {
		t.Fatal(err.Error())
	}
	dat, err := imports.Process(workDir, formatDat, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	output := strings.TrimSpace(string(dat))
	expected := strings.TrimSpace(expectedCodegen)
	t.Log(string(expected))
	t.Log(string(output))
	if output != expected {
		dmp := gdiff.New()
		diffs := dmp.DiffMain(expected, output, false)
		t.Fatal(dmp.DiffPrettyText(diffs))
	}
}
