//go:build !js

package bldr_plugin_compiler_go

import (
	"context"
	"go/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	vardef "github.com/s4wave/spacewave/bldr/plugin/vardef"
	gdiff "github.com/sergi/go-diff/diffmatchpatch"
	"github.com/sirupsen/logrus"
	"golang.org/x/tools/imports"
)

const expectedCodegen = `package main

import (
	"embed"
	"os"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	bldr_example "github.com/s4wave/spacewave/bldr/example"
	plugin_entrypoint "github.com/s4wave/spacewave/bldr/plugin/entrypoint"
	bldr_values "github.com/s4wave/spacewave/bldr/values"
	link_solicit_controller "github.com/s4wave/spacewave/net/link/solicit/controller"
	"github.com/sirupsen/logrus"
)

// StaticFS contains embedded static assets.
//
//go:embed config-set.bin
var StaticFS embed.FS

// PluginStartInfo contains the json-base64 encoded startup information.
var PluginStartInfo = strings.TrimSpace(os.Getenv("BLDR_PLUGIN_START_INFO"))

// PluginMeta contains the b58 encoded plugin metadata.
var PluginMeta = "L9hvzj3w1szjfbzYPQHjjHRhdViHQuZ28DTVYmQqWL1MozHowwx7YgCoJBpxQsoc3ieCX79XZcR"

// LogLevel is the default program log level.
var LogLevel = logrus.DebugLevel

// Factories are the factories included in the binary.
var Factories = []plugin_entrypoint.AddFactoryFunc{func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_example.NewFactory(b), boilerplate_controller.NewFactory(b), link_solicit_controller.NewFactory()}
}}

// ConfigSets are the configuration sets to apply on startup.
var ConfigSets = []plugin_entrypoint.BuildConfigSetFunc{plugin_entrypoint.ConfigSetFuncFromFS(StaticFS, "config-set.bin")}

// init sets variables at init time
func init() {
	bldr_example.AssetPath = "/path/to/asset.png"
}

// main is the main entrypoint.
func main() {
	plugin_entrypoint.Main(PluginStartInfo, PluginMeta, LogLevel, Factories, ConfigSets)
}

// _ ensures that at least one reference to bldr_values is present.
var _ bldr_values.VoidOutput
`

const expectedCodegenDevInfo = `package main

import (
	"embed"
	"os"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	bldr_example "github.com/s4wave/spacewave/bldr/example"
	plugin_entrypoint "github.com/s4wave/spacewave/bldr/plugin/entrypoint"
	bldr_values "github.com/s4wave/spacewave/bldr/values"
	link_solicit_controller "github.com/s4wave/spacewave/net/link/solicit/controller"
	"github.com/sirupsen/logrus"
)

// StaticFS contains embedded static assets.
//
//go:embed config-set.bin
var StaticFS embed.FS

// PluginStartInfo contains the json-base64 encoded startup information.
var PluginStartInfo = strings.TrimSpace(os.Getenv("BLDR_PLUGIN_START_INFO"))

// PluginMeta contains the b58 encoded plugin metadata.
var PluginMeta = "L9hvzj3w1szjfbzYPQHjjHRhdViHQuZ28DTVYmQqWL1MozHowwx7YgCoJBpxQsoc3ieCX79XZcR"

// LogLevel is the default program log level.
var LogLevel = logrus.DebugLevel

// Factories are the factories included in the binary.
var Factories = []plugin_entrypoint.AddFactoryFunc{func(b bus.Bus) []controller.Factory {
	return []controller.Factory{bldr_example.NewFactory(b), boilerplate_controller.NewFactory(b), link_solicit_controller.NewFactory()}
}}

// ConfigSets are the configuration sets to apply on startup.
var ConfigSets = []plugin_entrypoint.BuildConfigSetFunc{plugin_entrypoint.ConfigSetFuncFromFS(StaticFS, "config-set.bin")}

// init sets variables at init time
func init() {
	devInfo, err := plugin_entrypoint.PluginDevInfoFromFile("dev-info.bin")
	if err != nil {
		panic(err)
	}
	bldr_example.AssetPath = devInfo.LookupPluginDevVar("github.com/s4wave/spacewave/bldr/example", "AssetPath").GetStringValue()
}

// main is the main entrypoint.
func main() {
	plugin_entrypoint.Main(PluginStartInfo, PluginMeta, LogLevel, Factories, ConfigSets)
}

// _ ensures that at least one reference to bldr_values is present.
var _ bldr_values.VoidOutput
`

func TestCodegen(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	packagePaths := []string{
		"github.com/aperturerobotics/controllerbus/example/boilerplate/controller",
		"github.com/s4wave/spacewave/bldr/example",
		"github.com/s4wave/spacewave/net/link/solicit/controller",
	}

	workDir, _ := os.Getwd()
	workDir = filepath.Join(workDir, "../../../..")
	an, err := AnalyzePackages(ctx, le, workDir, packagePaths, []string{"build_type_dev"}, "", "")
	if err != nil {
		t.Fatal(err.Error())
	}
	pluginMeta := bldr_plugin.NewPluginMeta("test-project", "test-plugin", "desktop/linux/amd64", "dev")
	goVarDefs := []*vardef.PluginVar{vardef.NewPluginVar(
		"github.com/s4wave/spacewave/bldr/example",
		"AssetPath",
		&vardef.PluginVar_StringValue{StringValue: "/path/to/asset.png"},
	)}

	type testcase struct {
		fn     func() (*ast.File, error)
		result string
	}
	testcases := []*testcase{
		{
			fn: func() (*ast.File, error) {
				return CodegenPluginWrapperFromAnalysis(
					le,
					an,
					pluginMeta,
					[]string{"config-set.bin"},
					goVarDefs,
					"",
				)
			},
			result: expectedCodegen,
		},
		{
			fn: func() (*ast.File, error) {
				return CodegenPluginWrapperFromAnalysis(
					le,
					an,
					pluginMeta,
					[]string{"config-set.bin"},
					goVarDefs,
					"dev-info.bin",
				)
			},
			result: expectedCodegenDevInfo,
		},
	}
	for _, tcase := range testcases {
		genFile, err := tcase.fn()
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
		expected := strings.TrimSpace(tcase.result)
		t.Log("expected:")
		t.Log(expected)
		t.Log("actual:")
		t.Log(output)
		if output != expected {
			dmp := gdiff.New()
			diffs := dmp.DiffMain(expected, output, false)
			t.Fatal(dmp.DiffPrettyText(diffs))
		}
	}
}
