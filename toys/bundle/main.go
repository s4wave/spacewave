package main

// This is a basic prototype of the bundling process run against the repo root.

import (
	"context"
	"io/ioutil"
	"os"

	forge_lib_all "github.com/aperturerobotics/forge/lib/all"
	target_json "github.com/aperturerobotics/forge/target/json"
	"github.com/aperturerobotics/forge/testbed"
	"github.com/sirupsen/logrus"
)

func main() {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	if err := runBundleDemo(ctx, le); err != nil {
		os.Stderr.WriteString(err.Error())
		os.Stderr.WriteString("\n")
		os.Exit(1)
	}
}

// runBundleDemo runs the Execution demo.
func runBundleDemo(ctx context.Context, le *logrus.Entry) error {
	targetPath := "./bundle-target.yaml"
	if _, err := os.Stat(targetPath); err != nil {
		return err
	}
	targetData, err := ioutil.ReadFile(targetPath)
	if err != nil {
		return err
	}

	// unmarshal target from yaml into a container for later type resolution
	tb, err := testbed.Default(ctx)
	if err != nil {
		return err
	}
	forge_lib_all.AddFactories(tb.Bus, tb.StaticResolver)
	tgt, err := target_json.ResolveYAML(ctx, tb.Bus, targetData)
	if err != nil {
		return err
	}

	_, err = tb.RunExecutionWithTarget(tgt, nil)
	return err
}
