package main

import (
	"context"
	"path"
	"path/filepath"

	"github.com/aperturerobotics/bldr/runtime"
	"github.com/aperturerobotics/bldr/target/electron"
	"github.com/sirupsen/logrus"
)

// LogLevel is the default log level to use.
var LogLevel = logrus.DebugLevel

func main() {
	log := logrus.New()
	log.SetLevel(LogLevel)
	le := logrus.NewEntry(log)

	// get project root
	projRoot, err := filepath.Abs("../..")
	if err != nil {
		le.Fatal(err.Error())
	}
	binPath := path.Join(projRoot, "node_modules/.bin")
	electronPath := path.Join(binPath, "electron")
	electronRoot := path.Join(projRoot, "target/electron")
	// electronDevRoot := path.Join(projRoot, "target/electron-dev")
	rendererPath := path.Join(electronRoot, "build")

	// start with no webviews
	ctx := context.Background()

	// construct the electron WebView
	e, err := electron.RunElectron(ctx, le, electronPath, rendererPath)
	if err != nil {
		le.Fatal(err.Error())
	}

	rt, err := electron.NewRuntime(ctx, le, e)
	if err != nil {
		le.Fatal(err.Error())
	}
	if err := runtime.Run(ctx, le, rt); err != nil {
		le.Fatal(err.Error())
	}
}
