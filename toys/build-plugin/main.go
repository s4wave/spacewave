package main

import (
	"context"
	"os"
	"path"

	plugin_compiler "github.com/aperturerobotics/controllerbus/plugin/compiler"
	"github.com/sirupsen/logrus"
)

func main() {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	if err := run(ctx, le); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

func run(ctx context.Context, le *logrus.Entry) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	projRoot := "../../"
	projRoot = path.Join(wd, projRoot)

	// Compile as a plugin.
	// NOTE: this needs a new argument for wasm plugin, etc.
	return plugin_compiler.BuildPlugin(
		ctx, le,
		projRoot,
		"./runtime.cbus.so",
		// simple test: compile ourselves into a new plugin binary
		[]string{"github.com/aperturerobotics/bldr/toys/bundle"},
	)
}
