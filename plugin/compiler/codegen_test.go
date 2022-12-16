package plugin_compiler

import (
	"context"
	"go/ast"
	"go/token"
	"os"
	"path"
	"strings"
	"testing"

	gdiff "github.com/sergi/go-diff/diffmatchpatch"
	"github.com/sirupsen/logrus"
)

const expectedCodegen = `package main

import (
	"embed"
	bldr_example "github.com/aperturerobotics/bldr/example"
	"github.com/aperturerobotics/bldr/plugin/entrypoint"
	bldr_values "github.com/aperturerobotics/bldr/values"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
)
// AssetFS contains embedded static assets.
//
//go:embed config-set.bin
var AssetFS embed.FS
// Factories are the factories included in the binary.
var Factories = []plugin_entrypoint.AddFactoryFunc{func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_example.NewFactory(b), boilerplate_controller.NewFactory(b)}
}}
// ConfigSets are the configuration sets to apply on startup.
var ConfigSets = []plugin_entrypoint.BuildConfigSetFunc{plugin_entrypoint.ConfigSetFuncFromFS(AssetFS, "config-set.bin")}
// init sets variables at init time
func init() {
	bldr_example.ExampleScriptPath = "/path/to/script.js"
}
// main is the main entrypoint.
func main() {
	plugin_entrypoint.Main(Factories, ConfigSets)
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
	workDir = path.Join(workDir, "../..")
	an, err := AnalyzePackages(ctx, le, workDir, packagePaths)
	if err != nil {
		t.Fatal(err.Error())
	}
	genFile, err := GeneratePluginWrapper(
		le,
		an,
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
	dat, err := FormatFile(genFile)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Log(string(dat))
	output := strings.TrimSpace(string(dat))
	expected := strings.TrimSpace(expectedCodegen)
	if output != expected {
		dmp := gdiff.New()
		diffs := dmp.DiffMain(expected, output, false)
		t.Fatal(dmp.DiffPrettyText(diffs))
	}
}
