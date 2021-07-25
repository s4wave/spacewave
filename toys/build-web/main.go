package main

import (
	"context"
	"os"
	"path"

	"github.com/aperturerobotics/controllerbus/util/exec"
	"github.com/sirupsen/logrus"
	// esbuild "github.com/evanw/esbuild/pkg/api"
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

	// TODO: detect snowpack, webpack, etc.

	// Step: build the app using the users' bundler
	le.Info("calling yarn build...")
	ecmd := exec.NewCmd("yarn", "build")
	ecmd.Dir = projRoot
	err = exec.StartAndWait(ctx, le, ecmd)
	if err != nil {
		return err
	}

	le.Info("loading built assets...")
	// outDir := path.Join(projRoot, "build")

	return err
}
