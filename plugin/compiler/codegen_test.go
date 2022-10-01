package plugin_compiler

import (
	"context"
	"os"
	"strings"
	"testing"

	gdiff "github.com/sergi/go-diff/diffmatchpatch"
	"github.com/sirupsen/logrus"
)

const expectedCodegen = `//go:build bldr_plugin

package main

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	"github.com/aperturerobotics/controllerbus/plugin"
)
// Factories are the factories included in the binary.
var Factories = []plugin_entrypoint.AddFactoryFunc{func(b bus.Bus) []controller.Factory {
	return []controller.Factory{boilerplate_controller.NewFactory(b)}
}}
// ConfigSets are the configuration sets to apply on startup.
var ConfigSets []plugin_entrypoint.BuildConfigSetFunc
// main is the main entrypoint.
func main() {
	plugin_entrypoint.Main(Factories, ConfigSets)
}
`

func TestCodegen(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	packagePaths := []string{
		"github.com/aperturerobotics/controllerbus/example/boilerplate/controller",
	}
	workDir, _ := os.Getwd()
	an, err := AnalyzePackages(ctx, le, workDir, packagePaths)
	if err != nil {
		t.Fatal(err.Error())
	}
	genFile, err := GeneratePluginWrapper(le, an)
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
