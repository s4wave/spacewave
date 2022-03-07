package main

// This is a basic prototype of the bundling process run against the repo root.

import (
	"context"
	"io/ioutil"
	"os"

	forge_lib_all "github.com/aperturerobotics/forge/lib/all"
	forge_target "github.com/aperturerobotics/forge/target"
	target_json "github.com/aperturerobotics/forge/target/json"
	"github.com/aperturerobotics/forge/testbed"
	"github.com/aperturerobotics/timestamp"
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

	taskMap, err := target_json.ResolveTargetMapYAML(ctx, tb.Bus, targetData)
	if err != nil {
		return err
	}

	tts := timestamp.Now()
	valueSet := &forge_target.ValueSet{}
	job, err := tb.RunWorkerWithTasks(taskMap, valueSet, 1, &tts)
	if err != nil {
		return err
	}
	le.Infof("job complete: %v", job)
	return nil
}
